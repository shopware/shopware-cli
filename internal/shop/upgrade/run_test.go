package upgrade

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/executor"
)

// fakeExecutor satisfies executor.Executor and lets each test decide which
// shell command backs composer/php invocations.
type fakeExecutor struct {
	composer func(ctx context.Context, args ...string) *executor.Process
	php      func(ctx context.Context, args ...string) *executor.Process
	// env records the last WithEnv call so tests can assert the variables
	// composer/php invocations run with.
	env map[string]string
}

func shellProcess(ctx context.Context, script string) *executor.Process {
	return &executor.Process{Cmd: exec.CommandContext(ctx, "sh", "-c", script)}
}

func (f *fakeExecutor) ComposerCommand(ctx context.Context, args ...string) *executor.Process {
	return f.composer(ctx, args...)
}

func (f *fakeExecutor) PHPCommand(ctx context.Context, args ...string) *executor.Process {
	return f.php(ctx, args...)
}

func (f *fakeExecutor) ConsoleCommand(ctx context.Context, args ...string) *executor.Process {
	return shellProcess(ctx, "true")
}

func (f *fakeExecutor) NPMCommand(ctx context.Context, args ...string) *executor.Process {
	return shellProcess(ctx, "true")
}

func (f *fakeExecutor) NormalizePath(hostPath string) string { return hostPath }
func (f *fakeExecutor) Type() string                         { return executor.TypeLocal }
func (f *fakeExecutor) WithEnv(env map[string]string) executor.Executor {
	f.env = env
	return f
}
func (f *fakeExecutor) WithRelDir(string) executor.Executor             { return f }
func (f *fakeExecutor) StartEnvironment(context.Context) error          { return nil }
func (f *fakeExecutor) StopEnvironment(context.Context) error           { return nil }
func (f *fakeExecutor) EnvironmentStatus(context.Context) (bool, error) { return true, nil }
func (f *fakeExecutor) AdminAPIClient(context.Context) (*adminSdk.Client, error) {
	return nil, executor.ErrNotSupported
}

func runnerOptions() RunOptions {
	return RunOptions{
		Target: "6.7.11.0",
		Report: ReportData{
			ProjectName: "test",
			Current:     "6.6.10.3",
			Target:      "6.7.11.0",
		},
	}
}

func collectEvents(t *testing.T, events <-chan StepEvent) []StepEvent {
	t.Helper()
	var all []StepEvent
	for ev := range events {
		all = append(all, ev)
	}
	return all
}

func finalEvent(t *testing.T, events []StepEvent) StepEvent {
	t.Helper()
	require.NotEmpty(t, events)
	final := events[len(events)-1]
	require.Equal(t, StepFinished, final.Step)
	return final
}

func stepState(events []StepEvent, id StepID) (CheckState, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Step == id && events[i].Line == "" {
			return events[i].State, true
		}
	}
	return StatePending, false
}

func TestRunnerSuccess(t *testing.T) {
	dir := setupProject(t)
	u := NewProjectUpgrader(dir, &fakeExecutor{
		composer: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "echo composer "+args[0])
		},
		php: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "echo deployment helper")
		},
	})

	events := collectEvents(t, u.Run(t.Context(), runnerOptions()))

	final := finalEvent(t, events)
	assert.Equal(t, StateOK, final.State)
	require.NoError(t, final.Err)

	for _, id := range RunSteps {
		state, found := stepState(events, id)
		assert.True(t, found, "step %s has a completion event", id.Label())
		assert.Equal(t, StateOK, state, "step %s succeeded", id.Label())
	}

	// composer.json was rewritten to the target.
	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `"shopware/core": "6.7.11.0"`)

	// Streamed output reached the events and the log file.
	var lines []string
	for _, ev := range events {
		if ev.Line != "" {
			lines = append(lines, ev.Line)
		}
	}
	assert.Contains(t, lines, "composer update")

	logContent, err := os.ReadFile(u.LogPath())
	require.NoError(t, err)
	assert.Contains(t, string(logContent), "composer update")

	// The report was written with the composer changes.
	report, err := os.ReadFile(u.ReportPath())
	require.NoError(t, err)
	assert.Contains(t, string(report), "shopware/core: 6.6.10.3 -> 6.7.11.0")
}

func TestRunnerRestoresOnFailure(t *testing.T) {
	dir := setupProject(t)
	original, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)

	u := NewProjectUpgrader(dir, &fakeExecutor{
		composer: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "echo resolving; echo boom >&2; exit 2")
		},
		php: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "echo unreachable")
		},
	})

	events := collectEvents(t, u.Run(t.Context(), runnerOptions()))

	final := finalEvent(t, events)
	assert.Equal(t, StateFail, final.State)
	require.Error(t, final.Err)
	assert.Contains(t, final.Err.Error(), "composer update")

	state, _ := stepState(events, StepComposerUpdate)
	assert.Equal(t, StateFail, state)

	// The deployment helper never ran.
	_, found := stepState(events, StepDeploymentHelper)
	assert.False(t, found)

	// composer.json is back to its pre-upgrade content.
	restored, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, string(original), string(restored))

	// A failure report exists so the done screen's link points at something.
	report, err := os.ReadFile(u.ReportPath())
	require.NoError(t, err, "failed runs must still write a report")
	assert.Contains(t, string(report), "failed and rolled back")
	assert.Contains(t, string(report), "composer update")

	var restoreNote bool
	for _, ev := range events {
		if strings.Contains(ev.Line, "Restored composer.json") {
			restoreNote = true
		}
	}
	assert.True(t, restoreNote, "user is told about the rollback")
}

func TestRunnerRecipesInstallIsNonFatal(t *testing.T) {
	dir := setupProject(t)
	u := NewProjectUpgrader(dir, &fakeExecutor{
		composer: func(ctx context.Context, args ...string) *executor.Process {
			if args[0] == "symfony:recipes:install" {
				return shellProcess(ctx, "echo recipes install failed >&2; exit 1")
			}
			return shellProcess(ctx, "echo composer "+args[0])
		},
		php: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "echo deployment helper")
		},
	})

	events := collectEvents(t, u.Run(t.Context(), runnerOptions()))

	final := finalEvent(t, events)
	assert.Equal(t, StateOK, final.State, "recipes:install failure does not abort the upgrade")

	state, _ := stepState(events, StepRecipesInstall)
	assert.Equal(t, StateWarn, state)

	dhState, _ := stepState(events, StepDeploymentHelper)
	assert.Equal(t, StateOK, dhState, "later steps still ran")

	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `"shopware/core": "6.7.11.0"`, "no rollback happened")
}

func TestRunnerCancelledContext(t *testing.T) {
	dir := setupProject(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	u := NewProjectUpgrader(dir, &fakeExecutor{
		composer: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "echo composer")
		},
		php: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "echo php")
		},
	})

	events := collectEvents(t, u.Run(ctx, runnerOptions()))

	final := finalEvent(t, events)
	assert.Equal(t, StateFail, final.State)
	assert.ErrorIs(t, final.Err, context.Canceled)
}

func TestRunnerAppliesNoSecurityBlockingEnv(t *testing.T) {
	dir := setupProject(t)
	fake := &fakeExecutor{
		composer: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "true")
		},
		php: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "true")
		},
	}
	u := NewProjectUpgrader(dir, fake)
	u.DisableAuditBlock()

	events := collectEvents(t, u.Run(t.Context(), runnerOptions()))
	require.Equal(t, StateOK, finalEvent(t, events).State)

	assert.Equal(t, "1", fake.env["COMPOSER_NO_SECURITY_BLOCKING"],
		"the audit opt-out runs Composer with security blocking disabled")
}

func TestRunnerWithoutOptOutKeepsSecurityBlocking(t *testing.T) {
	dir := setupProject(t)
	fake := &fakeExecutor{
		composer: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "true")
		},
		php: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "true")
		},
	}
	u := NewProjectUpgrader(dir, fake)

	events := collectEvents(t, u.Run(t.Context(), runnerOptions()))
	require.Equal(t, StateOK, finalEvent(t, events).State)

	assert.NotContains(t, fake.env, "COMPOSER_NO_SECURITY_BLOCKING")
}

func TestCheckComposerResolvableNoAuditEnv(t *testing.T) {
	dir := setupProject(t)
	fake := &fakeExecutor{
		composer: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "true")
		},
		php: func(ctx context.Context, args ...string) *executor.Process {
			return shellProcess(ctx, "true")
		},
	}
	u := NewProjectUpgrader(dir, fake)
	u.DisableAuditBlock()

	_, err := u.CheckComposerResolvable(t.Context(), "6.7.11.0")
	require.NoError(t, err)

	assert.Equal(t, upgradeManifestName, fake.env["COMPOSER"])
	assert.Equal(t, "1", fake.env["COMPOSER_NO_SECURITY_BLOCKING"])
}

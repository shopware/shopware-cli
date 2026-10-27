package upgrade

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/tracking"
)

// headlessUpgrader builds an upgrader whose network lookups are stubbed and
// whose composer invocations run composerScript (version probes always
// succeed).
func headlessUpgrader(t *testing.T, dir, composerScript string) *ProjectUpgrader {
	t.Helper()

	exec := &fakeExecutor{
		composer: func(ctx context.Context, args ...string) *executor.Process {
			if len(args) > 0 && args[0] == "--version" {
				return shellProcess(ctx, "true")
			}
			return shellProcess(ctx, composerScript)
		},
		php: func(ctx context.Context, _ ...string) *executor.Process { return shellProcess(ctx, "true") },
	}

	u := NewProjectUpgrader(dir, exec)
	u.shopwareVersions = func(context.Context) ([]string, error) {
		return []string{"6.6.10.19", "6.7.11.0"}, nil
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(server.Close)
	u.endOfLifeURL = server.URL
	u.repositories = func(*composer.Json, *composer.Auth) *repository.Set {
		return repository.NewSet()
	}
	u.extensionUpdates = func(context.Context, string, string, []account_api.UpdateCheckExtension) ([]account_api.UpdateCheckExtensionCompatibility, error) {
		return nil, nil
	}
	u.storePlugins = func(context.Context, string, string, []string) ([]account_api.StorePlugin, error) {
		return nil, nil
	}
	return u
}

// setupComposerProject returns a project whose extensions are all
// Composer-managed — the readiness gate requires that.
func setupComposerProject(t *testing.T) string {
	t.Helper()
	dir := setupProject(t)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, "custom")))
	return dir
}

func headlessCatalog() *Catalog {
	return &Catalog{
		Current: version.Must(version.NewVersion("6.6.10.3")),
		Options: []VersionOption{
			{Version: version.Must(version.NewVersion("6.7.11.0")), Tag: "recommended"},
			{Version: version.Must(version.NewVersion("6.6.10.19")), Tag: "latest 6.6 patch"},
		},
		Recommended: 0,
		LatestPatch: 1,
	}
}

func TestSelectTarget(t *testing.T) {
	catalog := headlessCatalog()

	_, err := selectTarget(catalog, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--target is required")
	assert.Contains(t, err.Error(), "6.7.11.0  (recommended)")
	assert.Contains(t, err.Error(), "--target recommended / --target latest-patch")

	opt, err := selectTarget(catalog, TargetRecommended)
	require.NoError(t, err)
	assert.Equal(t, "6.7.11.0", opt.Version.String())

	opt, err = selectTarget(catalog, TargetLatestPatch)
	require.NoError(t, err)
	assert.Equal(t, "6.6.10.19", opt.Version.String())

	opt, err = selectTarget(catalog, "v6.7.11.0")
	require.NoError(t, err)
	assert.Equal(t, "6.7.11.0", opt.Version.String(), "a v prefix is accepted")

	_, err = selectTarget(catalog, "6.5.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a supported upgrade target")
}

func TestSelectTargetRejectsInvalidCatalogIndexes(t *testing.T) {
	catalog := headlessCatalog()
	catalog.Recommended = len(catalog.Options)
	_, err := selectTarget(catalog, TargetRecommended)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no recommended version")

	catalog.LatestPatch = len(catalog.Options) + 1
	_, err = selectTarget(catalog, TargetLatestPatch)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no newer patch")
}

func TestConsumeRunEventsReturnsFailureThatAlsoHasOutput(t *testing.T) {
	runErr := errors.New("upgrade failed")
	events := make(chan StepEvent, 1)
	events <- StepEvent{Step: StepFinished, State: StateFail, Line: "rollback detail", Err: runErr}
	close(events)

	var out bytes.Buffer
	failedStep, err := (&ProjectUpgrader{}).consumeRunEvents(&out, events)

	assert.ErrorIs(t, err, runErr)
	assert.Empty(t, failedStep, "no step failed before the synthetic finish")
	assert.Contains(t, out.String(), "rollback detail")
}

func TestConsumeRunEventsReturnsFirstFailingStep(t *testing.T) {
	runErr := errors.New("composer update failed")
	events := make(chan StepEvent, 4)
	events <- StepEvent{Step: StepRewriteComposer, State: StateOK}
	events <- StepEvent{Step: StepComposerUpdate, State: StateFail, Err: runErr}
	events <- StepEvent{Step: StepRecipesInstall, State: StateFail, Err: errors.New("skipped")}
	events <- StepEvent{Step: StepFinished, State: StateFail, Err: runErr}
	close(events)

	failedStep, err := (&ProjectUpgrader{}).consumeRunEvents(io.Discard, events)

	assert.ErrorIs(t, err, runErr)
	assert.Equal(t, "composer_update", failedStep)
}

// captureUpgradeEvents replaces the tracking seam and returns the recorded tags.
func captureUpgradeEvents(t *testing.T) *[]map[string]string {
	t.Helper()
	original := trackEvent
	t.Cleanup(func() { trackEvent = original })

	events := &[]map[string]string{}
	trackEvent = func(_ context.Context, event string, tags map[string]string) {
		assert.Equal(t, tracking.EventProjectUpgrade, event)
		*events = append(*events, tags)
	}
	return events
}

func TestRunHeadlessRequiresTarget(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	events := captureUpgradeEvents(t)
	dir := setupComposerProject(t)
	u := headlessUpgrader(t, dir, "true")

	var out bytes.Buffer
	err := u.RunHeadless(t.Context(), HeadlessOptions{Out: &out})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "--target is required")
	assert.Contains(t, ansi.Strip(out.String()), "Readiness checks")

	require.Len(t, *events, 1)
	assert.Equal(t, tracking.ResultFailure, (*events)[0][tracking.TagResult])
	assert.Equal(t, "target", (*events)[0][tracking.TagFailedStep])
	assert.Equal(t, "6.6.10.3", (*events)[0][tracking.TagFromVersion])
	assert.Empty(t, (*events)[0][tracking.TagTargetVersion])
}

func TestRunHeadlessDryRun(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	events := captureUpgradeEvents(t)
	dir := setupComposerProject(t)
	before, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)

	u := headlessUpgrader(t, dir, "true")

	var out bytes.Buffer
	err = u.RunHeadless(t.Context(), HeadlessOptions{Target: "6.7.11.0", DryRun: true, Out: &out})
	require.NoError(t, err)

	assert.Contains(t, ansi.Strip(out.String()), "Upgrade Shopware 6.6.10.3 -> 6.7.11.0")
	assert.Contains(t, ansi.Strip(out.String()), "Composer can resolve this upgrade.")
	assert.Contains(t, ansi.Strip(out.String()), "Planned changes (dry run — nothing modified)")
	assert.Contains(t, ansi.Strip(out.String()), "composer update --with-all-dependencies")

	after, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "a dry run must not modify the project")

	_, err = os.Stat(u.ReportPath())
	assert.NoError(t, err, "the dry run writes the report")
	assert.Empty(t, *events, "a dry run is not an upgrade attempt")
}

func TestRunHeadlessSecurityBlockedNeedsOptIn(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	events := captureUpgradeEvents(t)
	dir := setupComposerProject(t)
	u := headlessUpgrader(t, dir, "echo 'these were not loaded, because they are affected by security advisories'; exit 2")

	var out bytes.Buffer
	err := u.RunHeadless(t.Context(), HeadlessOptions{Target: "6.7.11.0", Out: &out})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "re-run with --no-audit")
	assert.Contains(t, err.Error(), "Shopware Security plugin")
	assert.False(t, u.AuditBlockDisabled())

	require.Len(t, *events, 1)
	assert.Equal(t, tracking.ResultFailure, (*events)[0][tracking.TagResult])
	assert.Equal(t, "security_advisory", (*events)[0][tracking.TagFailedStep])
	assert.Equal(t, "6.7.11.0", (*events)[0][tracking.TagTargetVersion])
}

func TestRunHeadlessInterruptedUpdateReportsCancelled(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	events := captureUpgradeEvents(t)
	dir := setupComposerProject(t)
	u := headlessUpgrader(t, dir, "true")

	// The preflight resolution passes; the context is cancelled as soon as
	// the runner asks for the real `composer update`, like a Ctrl+C there.
	ctx, cancel := context.WithCancel(t.Context())
	u.executor.(*fakeExecutor).composer = func(ctx context.Context, args ...string) *executor.Process {
		if (len(args) > 0 && args[0] == "--version") || slices.Contains(args, "--no-install") {
			return shellProcess(ctx, "true")
		}
		cancel()
		return shellProcess(ctx, "sleep 10")
	}

	var out bytes.Buffer
	err := u.RunHeadless(ctx, HeadlessOptions{Target: "6.7.11.0", Out: &out})

	require.Error(t, err)
	require.Len(t, *events, 1)
	assert.Equal(t, tracking.ResultCancelled, (*events)[0][tracking.TagResult])
	assert.Equal(t, "composer_update", (*events)[0][tracking.TagFailedStep])
	assert.Equal(t, "6.7.11.0", (*events)[0][tracking.TagTargetVersion])
}

func TestRunHeadlessExecutes(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	events := captureUpgradeEvents(t)
	dir := setupComposerProject(t)
	u := headlessUpgrader(t, dir, "true")

	var out bytes.Buffer
	err := u.RunHeadless(t.Context(), HeadlessOptions{Target: "6.7.11.0", Out: &out})
	require.NoError(t, err)

	assert.Contains(t, ansi.Strip(out.String()), "Executing upgrade")
	assert.Contains(t, ansi.Strip(out.String()), "Upgraded to Shopware 6.7.11.0.")

	content, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `"shopware/core": "6.7.11.0"`, "composer.json is rewritten to the target")

	_, err = os.Stat(u.ReportPath())
	assert.NoError(t, err)
	_, err = os.Stat(u.LogPath())
	assert.NoError(t, err)

	require.Len(t, *events, 1)
	assert.Equal(t, tracking.ResultSuccess, (*events)[0][tracking.TagResult])
	assert.NotContains(t, (*events)[0], tracking.TagFailedStep)
}

func TestRunHeadlessFailingUpdateRollsBack(t *testing.T) {
	t.Setenv("DO_NOT_TRACK", "1")
	events := captureUpgradeEvents(t)
	dir := setupComposerProject(t)
	before, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, err)

	// The preflight resolution must succeed while the real `composer update`
	// fails, so fail only when the runner's --with-all-dependencies run
	// executes without the --no-install preflight flag.
	u := headlessUpgrader(t, dir, `case "$*" in *--no-install*) exit 0;; *) echo boom >&2; exit 2;; esac`)
	u.executor.(*fakeExecutor).composer = func(ctx context.Context, args ...string) *executor.Process {
		if len(args) > 0 && args[0] == "--version" {
			return shellProcess(ctx, "true")
		}
		for _, a := range args {
			if a == "--no-install" {
				return shellProcess(ctx, "true")
			}
		}
		return shellProcess(ctx, "echo boom >&2; exit 2")
	}

	var out bytes.Buffer
	err = u.RunHeadless(t.Context(), HeadlessOptions{Target: "6.7.11.0", Out: &out})

	require.Error(t, err)
	assert.Contains(t, ansi.Strip(out.String()), "Upgrade failed and was rolled back.")

	after, readErr := os.ReadFile(filepath.Join(dir, "composer.json"))
	require.NoError(t, readErr)
	assert.Equal(t, string(before), string(after), "composer.json is restored after the failure")

	require.Len(t, *events, 1)
	assert.Equal(t, tracking.ResultFailure, (*events)[0][tracking.TagResult])
	assert.Equal(t, "composer_update", (*events)[0][tracking.TagFailedStep])
}

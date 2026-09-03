package dev

import (
	"context"
	"os/exec"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/executor"
)

type recordingInstallExecutor struct {
	executor.Executor
	consoleArgs [][]string
	envCalls    []map[string]string
	phpCalls    int
	failCommand string
	failPHP     bool
	phpOutput   string
}

func (e *recordingInstallExecutor) ConsoleCommand(ctx context.Context, args ...string) *executor.Process {
	e.consoleArgs = append(e.consoleArgs, args)
	if len(args) > 0 && args[0] == e.failCommand {
		return &executor.Process{Cmd: exec.CommandContext(ctx, "sh", "-c", "exit 1")}
	}
	return &executor.Process{Cmd: exec.CommandContext(ctx, "sh", "-c", "true")}
}

func (e *recordingInstallExecutor) WithEnv(env map[string]string) executor.Executor {
	e.envCalls = append(e.envCalls, env)
	return e
}

func (e *recordingInstallExecutor) PHPCommand(ctx context.Context, args ...string) *executor.Process {
	e.phpCalls++
	if e.failPHP {
		return &executor.Process{Cmd: exec.CommandContext(ctx, "sh", "-c", "exit 1")}
	}
	if e.phpOutput != "" {
		return &executor.Process{Cmd: exec.CommandContext(ctx, "printf", "%s\n", e.phpOutput)}
	}
	return &executor.Process{Cmd: exec.CommandContext(ctx, "sh", "-c", "true")}
}

func set(names ...string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, n := range names {
		m[n] = struct{}{}
	}
	return m
}

func TestAllRunning(t *testing.T) {
	t.Run("all defined services running", func(t *testing.T) {
		assert.True(t, allRunning(set("web", "database"), set("web", "database", "adminer")))
	})

	t.Run("a newly added service is not running", func(t *testing.T) {
		// worker was just added to compose.yaml but is not up yet.
		assert.False(t, allRunning(set("web", "database", "worker"), set("web", "database")))
	})

	t.Run("empty defined imposes no constraint", func(t *testing.T) {
		assert.True(t, allRunning(nil, set("web")))
	})
}

func TestRunShopwareInstallFrom_OnlyAttachesCredentialsToFullInstall(t *testing.T) {
	wizard := installWizard{
		CredentialStep: newInstallCredentialStep(),
		language:       "de-DE",
		currency:       "CHF",
	}
	wizard.SetUsername("admin")
	wizard.SetPassword("secret-password")

	t.Run("full install", func(t *testing.T) {
		execr := &recordingInstallExecutor{}
		m := Model{ctx: t.Context(), executor: execr, install: wizard}

		cmds := m.runShopwareInstallFrom(installStartStep)().(tea.BatchMsg)
		cmds[1]()

		assert.Equal(t, []map[string]string{{
			"INSTALL_LOCALE":         "de-DE",
			"INSTALL_CURRENCY":       "CHF",
			"INSTALL_ADMIN_USERNAME": "admin",
			"INSTALL_ADMIN_PASSWORD": "secret-password",
		}}, execr.envCalls)
	})

	t.Run("resumed install", func(t *testing.T) {
		execr := &recordingInstallExecutor{}
		m := Model{ctx: t.Context(), executor: execr, install: wizard}

		cmds := m.runShopwareInstallFrom("theme:change")().(tea.BatchMsg)
		cmds[1]()

		assert.Empty(t, execr.envCalls)
	})
}

func TestStreamInstallFrom_MidStepFinishesWithDeploymentHelper(t *testing.T) {
	execr := &recordingInstallExecutor{phpOutput: "helper output"}
	ch := make(chan string, 20)

	output, err := streamInstallFrom(t.Context(), execr, "theme:change", installWizard{}, "", ch)

	assert.NoError(t, err)
	assert.Equal(t, 1, execr.phpCalls)
	assert.Equal(t, [][]string{
		{"theme:change", "--all", "Storefront"},
		{"plugin:refresh"},
	}, execr.consoleArgs)
	assert.Equal(t, []string{
		"Start: theme:change",
		"Start: plugin:refresh",
		"helper output",
	}, output)
}

func TestStreamInstallFrom_StartRunsDeploymentHelper(t *testing.T) {
	execr := &recordingInstallExecutor{phpOutput: "helper output"}
	ch := make(chan string, 20)

	output, err := streamInstallFrom(t.Context(), execr, installStartStep, installWizard{}, "", ch)

	assert.NoError(t, err)
	assert.Equal(t, 1, execr.phpCalls)
	assert.Empty(t, execr.consoleArgs)
	assert.Equal(t, []string{"helper output"}, output)
}

func TestStreamInstallFrom_UserCreateCannotExposePasswordOnArgv(t *testing.T) {
	execr := &recordingInstallExecutor{}
	ch := make(chan string, 20)
	wizard := installWizard{}

	_, err := streamInstallFrom(t.Context(), execr, installUserCreateStep, wizard, "", ch)

	assert.ErrorContains(t, err, "cannot be retried safely")
	assert.Zero(t, execr.phpCalls)
	assert.Empty(t, execr.consoleArgs)
}

func TestStreamInstallFrom_StopsAfterFailedCommand(t *testing.T) {
	execr := &recordingInstallExecutor{failCommand: "theme:change"}
	ch := make(chan string, 20)

	_, err := streamInstallFrom(t.Context(), execr, "theme:change", installWizard{}, "", ch)

	assert.Error(t, err)
	assert.Zero(t, execr.phpCalls)
	assert.Equal(t, [][]string{
		{"theme:change", "--all", "Storefront"},
	}, execr.consoleArgs)
}

func TestStreamInstallFrom_ReturnsTrailingHelperFailure(t *testing.T) {
	execr := &recordingInstallExecutor{failPHP: true}
	ch := make(chan string, 20)

	_, err := streamInstallFrom(t.Context(), execr, "theme:change", installWizard{}, "", ch)

	assert.Error(t, err)
	assert.Equal(t, 1, execr.phpCalls)
	assert.Len(t, execr.consoleArgs, 2)
}

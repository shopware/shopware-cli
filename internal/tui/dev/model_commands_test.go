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
	envCalls  []map[string]string
	phpCalls  int
	failPHP   bool
	phpOutput string
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

func TestRunShopwareInstall_AttachesCredentials(t *testing.T) {
	wizard := installWizard{
		CredentialStep: newInstallCredentialStep(),
		language:       "de-DE",
		currency:       "CHF",
	}
	wizard.SetUsername("admin")
	wizard.SetPassword("secret-password")

	execr := &recordingInstallExecutor{}
	m := Model{ctx: t.Context(), executor: execr, install: wizard}

	cmds := m.runShopwareInstall()().(tea.BatchMsg)
	cmds[1]()

	assert.Equal(t, []map[string]string{{
		"INSTALL_LOCALE":         "de-DE",
		"INSTALL_CURRENCY":       "CHF",
		"INSTALL_ADMIN_USERNAME": "admin",
		"INSTALL_ADMIN_PASSWORD": "secret-password",
	}}, execr.envCalls)
}

func TestStreamInstall_RunsDeploymentHelper(t *testing.T) {
	execr := &recordingInstallExecutor{phpOutput: "helper output"}
	ch := make(chan string, 20)

	output, err := streamInstall(t.Context(), execr, ch)

	assert.NoError(t, err)
	assert.Equal(t, 1, execr.phpCalls)
	assert.Equal(t, []string{"helper output"}, output)
}

func TestStreamInstall_ReturnsHelperFailure(t *testing.T) {
	execr := &recordingInstallExecutor{failPHP: true}
	ch := make(chan string, 20)

	_, err := streamInstall(t.Context(), execr, ch)

	assert.Error(t, err)
	assert.Equal(t, 1, execr.phpCalls)
}

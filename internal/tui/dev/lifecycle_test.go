package dev

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
	"github.com/shopware/shopware-cli/internal/tui/prompt"
)

func newLifecycleModel(t *testing.T) Model {
	t.Helper()
	return Model{
		phase:       phaseStarting,
		projectRoot: t.TempDir(),
		config:      &shop.Config{},
		envConfig:   &shop.EnvironmentConfig{},
		watchers:    make(map[string]*watcherHandle),
	}
}

func TestUpdateLifecycle_DockerAlreadyRunning_AdvancesToDashboard(t *testing.T) {
	m := newLifecycleModel(t)

	updated, cmd := m.updateLifecycle(dockerAlreadyRunningMsg{})
	final := updated.(Model)
	assert.Equal(t, phaseDashboard, final.phase)
	assert.NotNil(t, cmd, "should kick off shopware install check")
}

func TestUpdateLifecycle_DockerNeedStart_TransitionsToStarting(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseDashboard
	m.overlayLines = []string{"stale"}
	m.dockerShowLogs = true

	updated, cmd := m.updateLifecycle(dockerNeedStartMsg{})
	final := updated.(Model)
	assert.Equal(t, phaseStarting, final.phase)
	assert.Empty(t, final.overlayLines)
	assert.False(t, final.dockerShowLogs)
	assert.NotNil(t, cmd, "should start containers")
}

func TestUpdateLifecycle_DockerOutputLine_AppendsToOverlay(t *testing.T) {
	m := newLifecycleModel(t)
	m.height = 40 // ensure overlayMaxLines is generous

	updated, _ := m.updateLifecycle(dockerOutputLineMsg("first line"))
	final := updated.(Model)
	assert.Equal(t, []string{"first line"}, final.overlayLines)
}

func TestUpdateLifecycle_DockerOutputLine_TruncatesToMaxLines(t *testing.T) {
	m := newLifecycleModel(t)
	m.height = 0 // overlayMaxLines() returns 10 in this case
	// Pre-fill with 10 lines so the next append forces a truncation.
	for i := 0; i < 10; i++ {
		m.overlayLines = append(m.overlayLines, "old")
	}

	updated, _ := m.updateLifecycle(dockerOutputLineMsg("new"))
	final := updated.(Model)
	assert.Len(t, final.overlayLines, 10)
	assert.Equal(t, "new", final.overlayLines[len(final.overlayLines)-1])
}

func TestUpdateLifecycle_DockerOutputLine_InstallingAdvancesProgress(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseInstalling
	m.height = 40
	m.installProg = installProgress{progress: newInstallProgress()}

	updated, cmd := m.updateLifecycle(dockerOutputLineMsg("Start: bin/console system:install --create-database"))
	final := updated.(Model)
	assert.Equal(t, 0, final.installProg.currentStep)
	assert.NotNil(t, cmd)
}

func TestUpdateLifecycle_DockerOutputDone_NoOp(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseStarting

	updated, cmd := m.updateLifecycle(dockerOutputDoneMsg{})
	final := updated.(Model)
	assert.Equal(t, phaseStarting, final.phase)
	assert.Nil(t, cmd)
}

func TestUpdateLifecycle_DockerStarted_Success(t *testing.T) {
	m := newLifecycleModel(t)
	m.overlayLines = []string{"some", "output"}
	ch := make(chan string)
	close(ch)
	m.dockerOutChan = ch

	updated, cmd := m.updateLifecycle(dockerStartedMsg{})
	final := updated.(Model)
	assert.Equal(t, phaseDashboard, final.phase)
	assert.Empty(t, final.overlayLines)
	assert.Nil(t, final.dockerOutChan)
	assert.NotNil(t, cmd, "should trigger install check after start")
}

func TestUpdateLifecycle_DockerStarted_ErrorStaysAndRendersError(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseStarting

	wantErr := errors.New("docker daemon not running")
	updated, cmd := m.updateLifecycle(dockerStartedMsg{err: wantErr})
	final := updated.(Model)

	// Phase stays on starting so the error overlay remains visible.
	assert.Equal(t, phaseStarting, final.phase)
	assert.True(t, final.dockerShowLogs, "logs view must be forced on so the error is visible without pressing 'l'")
	assert.Nil(t, cmd)
	joined := strings.Join(final.overlayLines, "\n")
	assert.Contains(t, joined, "Failed:")
	assert.Contains(t, joined, "docker daemon not running")
	assert.Contains(t, joined, "Press q to exit")
}

func TestUpdateLifecycle_ShopwareInstalled_StartsDashboard(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseStarting

	updated, cmd := m.updateLifecycle(shopwareInstalledMsg{})
	final := updated.(Model)
	assert.Equal(t, phaseDashboard, final.phase)
	assert.NotNil(t, cmd)
}

func TestUpdateLifecycle_ShopwareNotInstalled_OpensInstallPrompt(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseStarting
	m.overlayLines = []string{"docker logs"}

	updated, cmd := m.updateLifecycle(shopwareNotInstalledMsg{})
	final := updated.(Model)
	assert.Equal(t, phaseInstallPrompt, final.phase)
	assert.Empty(t, final.overlayLines)
	assert.Nil(t, cmd)

	assert.Equal(t, installStepAsk, final.install.step)
	assert.True(t, final.install.confirmYes)
	assert.True(t, final.install.PasswordMasked(), "password must start masked")
}

func TestUpdateLifecycle_ShopwareInstallDone_Success(t *testing.T) {
	dir := t.TempDir()

	creds := tui.NewCredentialStep(tui.CredentialStepOptions{Username: "myadmin", Password: "supersecret"})

	m := Model{
		phase:       phaseInstalling,
		projectRoot: dir,
		config:      &shop.Config{URL: "http://127.0.0.1:8000"},
		envConfig:   &shop.EnvironmentConfig{},
		watchers:    make(map[string]*watcherHandle),
		install: installWizard{
			CredentialStep: creds,
			step:           installStepCredentials,
		},
		installProg: installProgress{progress: newInstallProgress()},
	}
	ch := make(chan string)
	close(ch)
	m.dockerOutChan = ch

	updated, cmd := m.updateLifecycle(shopwareInstallDoneMsg{})
	final := updated.(Model)

	assert.Equal(t, phaseDashboard, final.phase)
	assert.True(t, final.installProg.done)
	assert.Equal(t, len(installStepPatterns), final.installProg.currentStep)

	assert.NotNil(t, final.envConfig.AdminApi)
	assert.Equal(t, "myadmin", final.envConfig.AdminApi.Username)
	assert.Equal(t, "supersecret", final.envConfig.AdminApi.Password)

	assert.Equal(t, "myadmin", final.overview.username)
	assert.Equal(t, "supersecret", final.overview.password)

	assert.Empty(t, final.overlayLines)
	assert.Nil(t, final.dockerOutChan)
	assert.NotNil(t, cmd, "should kick off dashboard")
}

func TestUpdateLifecycle_ShopwareInstallDone_ErrorShowsLogs(t *testing.T) {
	m := Model{
		phase:       phaseInstalling,
		projectRoot: t.TempDir(),
		config:      &shop.Config{},
		envConfig:   &shop.EnvironmentConfig{},
		watchers:    make(map[string]*watcherHandle),
		install: installWizard{
			CredentialStep: tui.NewCredentialStep(tui.CredentialStepOptions{}),
		},
		installProg: installProgress{progress: newInstallProgress()},
	}

	wantErr := errors.New("migration failed")
	updated, cmd := m.updateLifecycle(shopwareInstallDoneMsg{err: wantErr})
	final := updated.(Model)

	// Stays on installing so the operator can read logs.
	assert.Equal(t, phaseInstalling, final.phase)
	assert.True(t, final.installProg.showLogs)
	assert.Nil(t, cmd)

	joined := strings.Join(final.overlayLines, "\n")
	assert.Contains(t, joined, "Installation failed:")
	assert.Contains(t, joined, "migration failed")
	assert.Contains(t, joined, "Press q to exit")

	// envConfig should NOT be mutated on error.
	assert.Nil(t, final.envConfig.AdminApi)
}

func TestUpdateLifecycle_DockerStopped_QuitsApp(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseStopping

	_, cmd := m.updateLifecycle(dockerStoppedMsg{})
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit, "dockerStoppedMsg must emit tea.QuitMsg")
}

func TestUpdateLifecycle_PortConflict_OpensPromptOverlay(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseStarting
	m.host = app.New(app.Options{DisableDefaultKeys: true})

	conflicts := []dockerpkg.PortConflict{
		{Definition: dockerpkg.PortDefinition{Key: shop.DockerPortWeb, Label: "Shop (Caddy)", Target: 8000, Default: 8000}, HostPort: 8000},
	}

	updated, _ := m.updateLifecycle(portConflictMsg{conflicts: conflicts})
	final := updated.(Model)

	assert.Equal(t, phasePortConflict, final.phase)
	assert.Equal(t, conflicts, final.portConflicts)
	overlay := final.host.(*app.App).TopOverlay()
	assert.NotNil(t, overlay, "must push the port-conflict prompt")
	assert.Equal(t, portConflictID, overlay.ID())
}

func TestUpdateLifecycle_PortFixDone_Success_StartsContainers(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseStarting

	updated, cmd := m.updateLifecycle(portFixDoneMsg{})
	final := updated.(Model)

	assert.Equal(t, phaseStarting, final.phase)
	assert.NotNil(t, cmd, "must start containers after the ports were remapped")
}

func TestUpdateLifecycle_PortFixDone_ErrorShowsLogs(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phaseStarting

	wantErr := errors.New("no free port")
	updated, cmd := m.updateLifecycle(portFixDoneMsg{err: wantErr})
	final := updated.(Model)

	assert.Equal(t, phaseStarting, final.phase)
	assert.True(t, final.dockerShowLogs)
	assert.Nil(t, cmd)
	joined := strings.Join(final.overlayLines, "\n")
	assert.Contains(t, joined, "Failed:")
	assert.Contains(t, joined, "no free port")
	assert.Contains(t, joined, "Press q to exit")
}

func TestHandlePortConflictResult_RandomStartsFix(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phasePortConflict
	m.configPath = filepath.Join(m.projectRoot, ".shopware-project.yml")
	m.portConflicts = []dockerpkg.PortConflict{
		{Definition: dockerpkg.PortDefinition{Key: shop.DockerPortWeb, Target: 8000, Default: 8000}, HostPort: 8000},
	}

	updated, cmd := m.handlePortConflictResult(prompt.ResultMsg{ID: portConflictID, Choice: portConflictRandom})
	final := updated.(Model)

	assert.Equal(t, phaseStarting, final.phase)
	assert.NotNil(t, cmd)
}

func TestHandlePortConflictResult_QuitEmitsQuit(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phasePortConflict

	_, cmd := m.handlePortConflictResult(prompt.ResultMsg{ID: portConflictID, Choice: portConflictQuit})
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestHandlePortConflictResult_DismissedIsNoOp(t *testing.T) {
	m := newLifecycleModel(t)
	m.phase = phasePortConflict

	updated, cmd := m.handlePortConflictResult(prompt.ResultMsg{ID: portConflictID})
	final := updated.(Model)
	assert.Equal(t, phasePortConflict, final.phase)
	assert.Nil(t, cmd)
}

func TestFixPortConflicts_PersistsAndRewritesCompose(t *testing.T) {
	dir := t.TempDir()
	writeMinimalComposeProject(t, dir)

	m := newLifecycleModel(t)
	m.projectRoot = dir
	m.configPath = filepath.Join(dir, ".shopware-project.yml")
	m.portConflicts = []dockerpkg.PortConflict{
		{Definition: dockerpkg.PortDefinition{Key: shop.DockerPortWeb, Service: "web", Label: "Shop (Caddy)", Target: 8000, Default: 8000}, HostPort: 8000},
	}

	msg := m.fixPortConflicts()()
	result, ok := msg.(portFixDoneMsg)
	assert.True(t, ok)
	assert.NoError(t, result.err)

	// The override is applied to the in-memory config...
	assert.NotZero(t, m.config.Docker.Ports[shop.DockerPortWeb])
	assert.NotEqual(t, 8000, m.config.Docker.Ports[shop.DockerPortWeb])

	// ...persisted to the local override file...
	localContent, err := os.ReadFile(filepath.Join(dir, ".shopware-project.local.yml"))
	assert.NoError(t, err)
	assert.Contains(t, string(localContent), "web:")

	// ...and written into the regenerated compose file.
	composeContent, err := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	assert.NoError(t, err)
	assert.Contains(t, string(composeContent), fmt.Sprintf("%d:8000", m.config.Docker.Ports[shop.DockerPortWeb]))
}

// writeMinimalComposeProject writes the composer.lock WriteComposeFile needs.
func writeMinimalComposeProject(t *testing.T, dir string) {
	t.Helper()
	lock := `{"packages": [{"name": "shopware/core", "version": "6.6.0.0"}], "packages-dev": []}`
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644))
}

func TestUpdateLifecycle_UnknownMsg_NoOp(t *testing.T) {
	m := newLifecycleModel(t)
	type unknownMsg struct{}

	updated, cmd := m.updateLifecycle(unknownMsg{})
	final := updated.(Model)
	assert.Equal(t, phaseStarting, final.phase)
	assert.Nil(t, cmd)
}

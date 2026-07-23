package devtui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
	"github.com/shopware/shopware-cli/internal/tui/picker"
	"github.com/shopware/shopware-cli/internal/tui/prompt"
)

// newTestModel returns a Model with the minimum fields populated to drive
// dashboard-phase key dispatch tests without nil-deref panics. The model is
// attached to an app shell so overlay pushes have a host; assert on open
// overlays with topOverlay.
func newTestModel() Model {
	m := Model{
		phase:       phaseDashboard,
		overview:    NewOverviewModel("local", "http://localhost:8000", "", "", "/tmp/project", nil, nil),
		instance:    NewInstanceModel("/tmp/project", false),
		configTab:   NewConfigModel(nil, nil),
		watchers:    make(map[string]*watcherHandle),
		projectRoot: "/tmp/project",
		config:      &shop.Config{},
	}
	m.host = app.New(app.Options{DisableDefaultKeys: true})
	return m
}

// topOverlay returns the overlay the model pushed onto its shell, or nil.
func topOverlay(m Model) app.Overlay {
	return m.host.(*app.App).TopOverlay()
}

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)})
}

func keyCtrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: r, Mod: tea.ModCtrl})
}

func keySpecial(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
}

func keyShiftTabMsg() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift})
}

func TestNew_InitializesFields(t *testing.T) {
	cfg := &shop.Config{URL: "http://example.test"}
	env := &shop.EnvironmentConfig{}
	opts := Options{
		ProjectRoot: "/tmp/x",
		Config:      cfg,
		EnvConfig:   env,
		Executor:    nil,
	}
	// Executor is nil — call Type via interface would panic, so this is a
	// guarded smoke: New dereferences opts.Executor.Type(). Skip if that's
	// the case. We construct a real local executor instead.
	exec := &executor.LocalExecutor{}
	opts.Executor = exec
	m := New(opts)

	assert.Equal(t, tabOverview, m.activeTab)
	assert.False(t, m.dockerMode)
	assert.NotNil(t, m.watchers)
	assert.Empty(t, m.watchers)
	assert.Equal(t, phaseDashboard, m.phase)
}

func TestNewMigrationWizard_StartsInMigrationWizardPhase(t *testing.T) {
	cfg := &shop.Config{}
	opts := Options{
		ProjectRoot: t.TempDir(),
		Config:      cfg,
		EnvConfig:   &shop.EnvironmentConfig{},
		Executor:    &executor.LocalExecutor{},
	}
	m := NewMigrationWizard(opts)
	assert.Equal(t, phaseMigrationWizard, m.phase)
	assert.True(t, m.dockerMode)
}

func TestInit_MigrationWizardPhaseReturnsNil(t *testing.T) {
	m := newTestModel()
	m.phase = phaseMigrationWizard
	assert.Nil(t, m.Init())
}

func TestUpdateKeyPress_PhaseStarting_QuitKey(t *testing.T) {
	m := newTestModel()
	m.phase = phaseStarting

	_, cmd := m.Update(keyRune('q'))
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestUpdateKeyPress_PhaseStarting_LTogglesLogs(t *testing.T) {
	m := newTestModel()
	m.phase = phaseStarting
	m.dockerShowLogs = false

	updated, _ := m.Update(keyRune('l'))
	assert.True(t, updated.(Model).dockerShowLogs)

	updated, _ = updated.(Model).Update(keyRune('l'))
	assert.False(t, updated.(Model).dockerShowLogs)
}

func TestUpdateKeyPress_PhaseStarting_OtherKeyIgnored(t *testing.T) {
	m := newTestModel()
	m.phase = phaseStarting

	updated, cmd := m.Update(keyRune('x'))
	assert.Nil(t, cmd)
	assert.False(t, updated.(Model).dockerShowLogs)
}

func TestUpdateKeyPress_PhaseStopping_CtrlCQuits(t *testing.T) {
	m := newTestModel()
	m.phase = phaseStopping

	_, cmd := m.Update(keyCtrl('c'))
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestUpdateKeyPress_PhaseInstalling_LTogglesLogs(t *testing.T) {
	m := newTestModel()
	m.phase = phaseInstalling
	m.installProg.showLogs = false

	updated, _ := m.Update(keyRune('l'))
	assert.True(t, updated.(Model).installProg.showLogs)
}

func TestUpdateKeyPress_PhaseInstalling_QuitKey(t *testing.T) {
	m := newTestModel()
	m.phase = phaseInstalling

	_, cmd := m.Update(keyRune('q'))
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestUpdateKeyPress_PhaseTask_DoneTransitionsToDashboard(t *testing.T) {
	m := newTestModel()
	m.phase = phaseTask
	m.task = tui.NewTask("Building...")
	m.task, _ = m.task.Update(tui.TaskDoneMsg{})

	updated, cmd := m.Update(keyRune('x'))
	assert.Nil(t, cmd)
	um := updated.(Model)
	assert.Equal(t, phaseDashboard, um.phase)
	assert.False(t, um.task.Done(), "task state is reset on dismissal")
}

func TestUpdateKeyPress_PhaseTask_NotDoneQuitOnQ(t *testing.T) {
	m := newTestModel()
	m.phase = phaseTask
	m.task = tui.NewTask("Building...")

	_, cmd := m.Update(keyRune('q'))
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestUpdateKeyPress_PhaseTask_NotDoneOtherKeyIgnored(t *testing.T) {
	m := newTestModel()
	m.phase = phaseTask
	m.task = tui.NewTask("Building...")

	updated, cmd := m.Update(keyRune('x'))
	assert.Nil(t, cmd)
	assert.Equal(t, phaseTask, updated.(Model).phase)
}

func TestUpdateKeyPress_PhaseInstallPrompt_Routed(t *testing.T) {
	m := newTestModel()
	m.phase = phaseInstallPrompt

	// Ctrl+C in install prompt should quit (per updateInstallPrompt)
	_, cmd := m.Update(keyCtrl('c'))
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestUpdateKeyPress_PhaseMigrationWizard_RoutesToMigrationWizard(t *testing.T) {
	m := newTestModel()
	m.phase = phaseMigrationWizard
	m.migrationWizard = newMigrationWizard("")
	// Welcome step: Enter with confirmYes=true advances to admin user step
	m.migrationWizard.confirmYes = true

	updated, _ := m.Update(keySpecial(tea.KeyEnter))
	assert.Equal(t, migrationStepAdminUser, updated.(Model).migrationWizard.step)
}

func TestUpdateDashboardKeys_CtrlPOpensPalette(t *testing.T) {
	m := newTestModel()

	updated, cmd := m.Update(keyCtrl('p'))
	um := updated.(Model)
	_, ok := topOverlay(um).(*commandPalette)
	assert.True(t, ok)
	assert.NotNil(t, cmd)
}

func TestUpdateDashboardKeys_DigitSwitchesTabs(t *testing.T) {
	m := newTestModel()

	updated, _ := m.Update(keyRune('2'))
	assert.Equal(t, tabInstance, updated.(Model).activeTab)

	updated, _ = updated.(Model).Update(keyRune('3'))
	assert.Equal(t, tabConfig, updated.(Model).activeTab)

	updated, _ = updated.(Model).Update(keyRune('1'))
	assert.Equal(t, tabOverview, updated.(Model).activeTab)
}

func TestUpdateDashboardKeys_TabCyclesForward(t *testing.T) {
	m := newTestModel()
	assert.Equal(t, tabOverview, m.activeTab)

	updated, _ := m.Update(keySpecial(tea.KeyTab))
	assert.Equal(t, tabInstance, updated.(Model).activeTab)

	updated, _ = updated.(Model).Update(keySpecial(tea.KeyTab))
	assert.Equal(t, tabConfig, updated.(Model).activeTab)

	updated, _ = updated.(Model).Update(keySpecial(tea.KeyTab))
	assert.Equal(t, tabOverview, updated.(Model).activeTab)
}

func TestUpdateDashboardKeys_ShiftTabCyclesBackward(t *testing.T) {
	m := newTestModel()

	updated, _ := m.Update(keyShiftTabMsg())
	assert.Equal(t, tabConfig, updated.(Model).activeTab)

	updated, _ = updated.(Model).Update(keyShiftTabMsg())
	assert.Equal(t, tabInstance, updated.(Model).activeTab)
}

func TestUpdateDashboardKeys_QuitWhenNotDockerQuits(t *testing.T) {
	m := newTestModel()
	m.dockerMode = false

	_, cmd := m.Update(keyRune('q'))
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestUpdateDashboardKeys_QuitDockerModeOpensConfirm(t *testing.T) {
	m := newTestModel()
	m.dockerMode = true

	updated, cmd := m.Update(keyRune('q'))
	um := updated.(Model)
	_, ok := topOverlay(um).(*prompt.Overlay)
	assert.True(t, ok)
	assert.Nil(t, cmd, "stop confirm has no init cmd")
}

func TestUpdateDashboardKeys_CtrlCDockerModeOpensConfirm(t *testing.T) {
	m := newTestModel()
	m.dockerMode = true

	updated, _ := m.Update(keyCtrl('c'))
	um := updated.(Model)
	_, ok := topOverlay(um).(*prompt.Overlay)
	assert.True(t, ok)
}

func TestUpdateConfigTab_EnterOnSaveWritesConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := &shop.Config{URL: "http://localhost:8000"}
	m := newTestModel()
	m.config = cfg
	m.projectRoot = dir
	m.activeTab = tabConfig
	m.configTab = NewConfigModel(cfg, nil)
	m.configTab.cursor = fieldSave
	m.configTab.modified = true
	m.configTab.phpVersion = 0

	updated, _ := m.Update(keySpecial(tea.KeyEnter))
	um := updated.(Model)
	assert.True(t, um.configTab.saved)
	assert.False(t, um.configTab.modified)
	assert.NoError(t, um.configTab.err)

	// File should exist on disk
	_, statErr := os.Stat(filepath.Join(dir, ".shopware-project.yml"))
	assert.NoError(t, statErr)
}

func TestUpdateConfigTab_EnterOnSaveFailureSetsErr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based unwritable directory not reliable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod will not produce write failure")
	}
	dir := t.TempDir()
	assert.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	cfg := &shop.Config{}
	m := newTestModel()
	m.config = cfg
	m.projectRoot = dir
	m.activeTab = tabConfig
	m.configTab = NewConfigModel(cfg, nil)
	m.configTab.cursor = fieldSave
	m.configTab.modified = true

	updated, _ := m.Update(keySpecial(tea.KeyEnter))
	um := updated.(Model)
	assert.Error(t, um.configTab.err)
	assert.False(t, um.configTab.saved)
}

func TestUpdateConfigTab_EnterOnPickerFieldOpensModal(t *testing.T) {
	m := newTestModel()
	m.activeTab = tabConfig
	m.configTab.cursor = fieldPHPVersion

	updated, cmd := m.Update(keySpecial(tea.KeyEnter))
	um := updated.(Model)
	_, ok := topOverlay(um).(*picker.Overlay)
	assert.True(t, ok)
	assert.NotNil(t, cmd)
}

func TestExecuteCommand_TabRouting(t *testing.T) {
	m := newTestModel()

	updated, _ := m.executeCommand("tab-instance")
	assert.Equal(t, tabInstance, updated.(Model).activeTab)

	updated, _ = updated.(Model).executeCommand("tab-config")
	assert.Equal(t, tabConfig, updated.(Model).activeTab)

	updated, _ = updated.(Model).executeCommand("tab-overview")
	assert.Equal(t, tabOverview, updated.(Model).activeTab)
}

func TestExecuteCommand_QuitNonDockerReturnsTeaQuit(t *testing.T) {
	m := newTestModel()
	m.dockerMode = false

	_, cmd := m.executeCommand("quit")
	assert.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)
}

func TestExecuteCommand_QuitDockerOpensStopConfirm(t *testing.T) {
	m := newTestModel()
	m.dockerMode = true

	updated, cmd := m.executeCommand("quit")
	um := updated.(Model)
	assert.Nil(t, cmd, "stop confirm has no init cmd")
	_, ok := topOverlay(um).(*prompt.Overlay)
	assert.True(t, ok)
}

func TestExecuteCommand_AdminWatchStartSetsStarting(t *testing.T) {
	m := newTestModel()
	m.overview.adminWatchRunning = false
	m.overview.adminWatchStarting = false

	updated, cmd := m.executeCommand("admin-watch-start")
	um := updated.(Model)
	assert.True(t, um.overview.adminWatchStarting)
	assert.NotNil(t, cmd)
}

func TestExecuteCommand_AdminWatchStartNoOpWhenRunning(t *testing.T) {
	m := newTestModel()
	m.overview.adminWatchRunning = true

	updated, cmd := m.executeCommand("admin-watch-start")
	um := updated.(Model)
	assert.False(t, um.overview.adminWatchStarting)
	assert.Nil(t, cmd)
}

func TestExecuteCommand_AdminWatchStopClearsRunning(t *testing.T) {
	m := newTestModel()
	m.overview.adminWatchRunning = true
	m.watchers[watcherAdmin] = &watcherHandle{}

	updated, cmd := m.executeCommand("admin-watch-stop")
	um := updated.(Model)
	assert.False(t, um.overview.adminWatchRunning)
	assert.NotNil(t, cmd)
	// stopWatcher deletes the entry from the map
	_, exists := um.watchers[watcherAdmin]
	assert.False(t, exists)
}

func TestStopWatcher_RemovesFromMapAndEmitsMsg(t *testing.T) {
	m := newTestModel()
	// Use a nil process entry so cmd() doesn't try to Stop a real exec.Cmd.
	m.watchers["test-watcher"] = nil

	cmd := m.stopWatcher("test-watcher")
	_, exists := m.watchers["test-watcher"]
	assert.False(t, exists, "watcher entry must be removed from map")
	assert.NotNil(t, cmd)

	msg := cmd()
	wm, ok := msg.(watcherStoppedMsg)
	assert.True(t, ok, "cmd should produce watcherStoppedMsg, got %T", msg)
	assert.Equal(t, "test-watcher", wm.name)
}

func TestStopWatcher_NoEntryStillEmitsMsg(t *testing.T) {
	m := newTestModel()

	cmd := m.stopWatcher("missing")
	msg := cmd()
	wm, ok := msg.(watcherStoppedMsg)
	assert.True(t, ok)
	assert.Equal(t, "missing", wm.name)
}

func TestMergeLocalProfilerSecrets_NilSrcIsNoOp(t *testing.T) {
	dst := &shop.Config{Docker: &shop.ConfigDocker{PHP: &shop.ConfigDockerPHP{Version: "8.4"}}}
	mergeLocalProfilerSecrets(dst, nil)
	assert.Equal(t, "8.4", dst.Docker.PHP.Version)
}

func TestMergeLocalProfilerSecrets_NilDockerOrPHPIsNoOp(t *testing.T) {
	dst := &shop.Config{Docker: &shop.ConfigDocker{PHP: &shop.ConfigDockerPHP{Version: "8.4"}}}

	mergeLocalProfilerSecrets(dst, &shop.Config{})
	assert.Equal(t, "8.4", dst.Docker.PHP.Version)
	assert.Empty(t, dst.Docker.PHP.BlackfireServerID)

	mergeLocalProfilerSecrets(dst, &shop.Config{Docker: &shop.ConfigDocker{}})
	assert.Empty(t, dst.Docker.PHP.BlackfireServerID)
}

func TestMergeLocalProfilerSecrets_CopiesAllSecrets(t *testing.T) {
	dst := &shop.Config{}
	src := &shop.Config{
		Docker: &shop.ConfigDocker{
			PHP: &shop.ConfigDockerPHP{
				BlackfireServerID:    "bf-id",
				BlackfireServerToken: "bf-token",
				TidewaysAPIKey:       "tw-key",
			},
		},
	}
	mergeLocalProfilerSecrets(dst, src)
	assert.NotNil(t, dst.Docker)
	assert.NotNil(t, dst.Docker.PHP)
	assert.Equal(t, "bf-id", dst.Docker.PHP.BlackfireServerID)
	assert.Equal(t, "bf-token", dst.Docker.PHP.BlackfireServerToken)
	assert.Equal(t, "tw-key", dst.Docker.PHP.TidewaysAPIKey)
}

func TestMergeLocalProfilerSecrets_EmptySrcValuesDoNotOverwriteDst(t *testing.T) {
	dst := &shop.Config{
		Docker: &shop.ConfigDocker{
			PHP: &shop.ConfigDockerPHP{
				BlackfireServerID: "existing",
			},
		},
	}
	src := &shop.Config{
		Docker: &shop.ConfigDocker{
			PHP: &shop.ConfigDockerPHP{
				BlackfireServerID:    "",
				BlackfireServerToken: "new-token",
			},
		},
	}
	mergeLocalProfilerSecrets(dst, src)
	assert.Equal(t, "existing", dst.Docker.PHP.BlackfireServerID)
	assert.Equal(t, "new-token", dst.Docker.PHP.BlackfireServerToken)
}

func TestView_DoesNotPanicForEachPhase(t *testing.T) {
	ctx := app.Context{Width: 120, Height: 40, MainHeight: 36}
	phases := []phase{phaseDashboard, phaseStarting, phaseStopping, phaseInstallPrompt, phaseInstalling, phaseTask, phaseMigrationWizard}
	for _, p := range phases {
		m := newTestModel()
		m.width = 120
		m.height = 40
		m.phase = p
		if p == phaseMigrationWizard {
			m.migrationWizard = newMigrationWizard("")
		}
		if p == phaseStarting || p == phaseStopping {
			m.dockerSpinner = tui.NewBrandSpinner()
		}
		if p == phaseInstalling {
			m.installProg.spinner = tui.NewBrandSpinner()
			m.installProg.progress = newInstallProgress()
		}

		assert.NotPanics(t, func() {
			_ = m.View(ctx)
			_ = m.chromeHeader(ctx)
			_ = m.chromeFooter(ctx)
		}, "phase %d", p)
	}
}

func TestView_ZeroSizeDoesNotPanic(t *testing.T) {
	m := newTestModel()
	assert.NotPanics(t, func() {
		_ = m.View(app.Context{})
	})
}

func TestView_StopConfirmOverlayRenders(t *testing.T) {
	m := newTestModel()
	m.dockerMode = true

	updated, _ := m.Update(keyCtrl('c'))
	um := updated.(Model)

	overlay := topOverlay(um)
	assert.NotNil(t, overlay)
	assert.NotPanics(t, func() {
		_ = overlay.View(120, 40)
	})
}

func TestSaveMigrationWizard_PersistsConfigToDisk(t *testing.T) {
	dir := t.TempDir()
	m := newTestModel()
	m.projectRoot = dir
	m.config = &shop.Config{}
	m.migrationWizard = newMigrationWizard(dir)
	m.migrationWizard.step = migrationStepReview

	updated, _ := m.saveMigrationWizard()
	um := updated.(Model)
	assert.NoError(t, um.migrationWizard.err)
	assert.Equal(t, migrationStepDone, um.migrationWizard.step)
	_, err := os.Stat(filepath.Join(dir, ".shopware-project.yml"))
	assert.NoError(t, err)
}

func TestSaveMigrationWizard_FailedWriteSetsErr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based unwritable directory not reliable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root — chmod will not produce write failure")
	}
	dir := t.TempDir()
	assert.NoError(t, os.Chmod(dir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	m := newTestModel()
	m.projectRoot = dir
	m.config = &shop.Config{}
	m.migrationWizard = newMigrationWizard("")

	updated, _ := m.saveMigrationWizard()
	um := updated.(Model)
	assert.Error(t, um.migrationWizard.err)
	assert.Equal(t, migrationStepDone, um.migrationWizard.step)
}

// TestUpdateChildren_KeyOnlyReachesActiveTab guards against keypresses meant for
// one tab leaking into the hidden tabs' handlers. With the Logs tab active,
// pressing Enter must not run the Overview tab's activate() logic.
func TestUpdateChildren_KeyOnlyReachesActiveTab(t *testing.T) {
	m := newTestModel()
	m.activeTab = tabInstance
	// Overview cursor sits on the Admin watcher (0); an Enter leaking through
	// would flip adminWatchStarting.
	m.overview.cursor = 0

	updated, _ := m.updateChildren(keySpecial(tea.KeyEnter))
	um := updated.(Model)

	assert.False(t, um.overview.adminWatchStarting, "Enter on the Logs tab must not activate the Overview watcher")
}

// TestUpdateChildren_KeyReachesActiveOverview confirms the active tab still
// receives its keys after the routing change.
func TestUpdateChildren_KeyReachesActiveOverview(t *testing.T) {
	m := newTestModel()
	m.activeTab = tabOverview
	m.overview.cursor = 0 // Admin watcher

	updated, cmd := m.updateChildren(keySpecial(tea.KeyEnter))
	um := updated.(Model)

	assert.True(t, um.overview.adminWatchStarting, "Enter on the Overview tab must activate the Admin watcher")
	assert.NotNil(t, cmd)
}

// TestStartStorefrontWatchRequest_OpensPicker verifies the Overview tab delegates
// storefront-watch start to the parent so the sales-channel picker resolves the
// theme/domain, instead of starting with empty options.
func TestStartStorefrontWatchRequest_OpensPicker(t *testing.T) {
	m := newTestModel()
	m.activeTab = tabOverview
	m.executor = &executor.LocalExecutor{}
	m.overview.cursor = 1 // Storefront watcher

	// Enter on the storefront row should emit startStorefrontWatchRequestMsg.
	updated, cmd := m.updateChildren(keySpecial(tea.KeyEnter))
	m = updated.(Model)
	if assert.NotNil(t, cmd) {
		_, ok := cmd().(startStorefrontWatchRequestMsg)
		assert.True(t, ok, "storefront activation must request the picker, not start directly")
	}
	assert.False(t, m.overview.sfWatchStarting, "watcher must not be marked starting before the picker resolves")

	// The parent handling that request opens the picker overlay.
	updated, _ = m.Update(startStorefrontWatchRequestMsg{})
	m = updated.(Model)
	_, ok := topOverlay(m).(*salesChannelPicker)
	assert.True(t, ok, "parent must open the sales-channel picker on the request")
}

func TestView_WindowTitlePerPhase(t *testing.T) {
	cases := []struct {
		phase     phase
		wantTitle string
	}{
		{phaseDashboard, "[project] · Overview"},
		{phaseStarting, "[project] · Starting..."},
		{phaseStopping, "[project] · Stopping"},
		{phaseInstallPrompt, "[project] · Install"},
		{phaseInstalling, "[project] · Installing..."},
		{phaseMigrationWizard, "[project] · Setup"},
	}

	for _, tc := range cases {
		m := newTestModel()
		m.projectRoot = "/tmp/project"
		m.phase = tc.phase

		assert.Equal(t, tc.wantTitle, m.windowTitle(), "phase %d", tc.phase)
	}
}

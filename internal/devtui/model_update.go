package devtui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tracking"
)

func (m Model) updateKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.modal != nil {
		next, cmd := m.modal.Update(msg)
		m.modal = next
		return m, cmd
	}

	if m.phase == phaseMigrationWizard {
		return m.updateMigrationWizard(msg)
	}

	if m.phase == phaseInstallPrompt {
		return m.updateInstallPrompt(msg)
	}

	if m.phase == phaseStarting || m.phase == phaseStopping {
		switch keyString(msg) {
		case "l":
			m.dockerShowLogs = !m.dockerShowLogs
		case keyQ, keyCtrlC:
			if m.phase == phaseStarting {
				if tags, ok := m.telemetry.dockerStartTags(nil); ok {
					tags[tracking.TagResult] = tracking.ResultCancelled
					trackEventNow(tracking.EventDevDockerStart, tags)
				}
			}
			return m, tea.Quit
		}
		return m, nil
	}

	if m.phase == phaseInstalling {
		switch keyString(msg) {
		case "l":
			m.installProg.showLogs = !m.installProg.showLogs
		case keyQ, keyCtrlC:
			if m.telemetry.installOnce() {
				tags := m.telemetry.installTags(tracking.ResultCancelled, m.install)
				tags[tracking.TagAbandonedAt] = "installing"
				trackEventNow(tracking.EventDevInstall, tags)
			}
			return m, tea.Quit
		}
		return m, nil
	}

	if m.phase == phaseTask {
		if m.taskDone {
			m.phase = phaseDashboard
			m.overlayLines = nil
			m.taskDone = false
			m.taskErr = nil
			return m, nil
		}
		if keyString(msg) == keyQ || keyString(msg) == keyCtrlC {
			if tags, ok := m.telemetry.taskTags(tracking.ResultCancelled); ok {
				trackEventNow(tracking.EventDevAction, tags)
			}
			return m, tea.Quit
		}
		return m, nil
	}

	return m.updateDashboardKeys(msg)
}

func (m Model) updateDashboardKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch keyString(msg) {
	case "ctrl+p":
		m.modal = newCommandPalette(paletteState{
			adminWatchActive: m.overview.adminWatchRunning || m.overview.adminWatchStarting,
			sfWatchActive:    m.overview.sfWatchRunning || m.overview.sfWatchStarting,
		})
		return m, textinput.Blink
	case keyCtrlC, keyQ:
		if m.dockerMode {
			m.modal = newStopConfirm()
			return m, nil
		}
		m.shutdown()
		return m, tea.Quit
	case key1:
		m.activeTab = tabOverview
		m.telemetry.markTab(m.activeTab)
		return m, nil
	case key2:
		m.activeTab = tabInstance
		m.telemetry.markTab(m.activeTab)
		return m, nil
	case key3:
		m.activeTab = tabConfig
		m.telemetry.markTab(m.activeTab)
		return m, nil
	case keyTab:
		m.activeTab = (m.activeTab + 1) % activeTab(len(tabNames))
		m.telemetry.markTab(m.activeTab)
		return m, nil
	case keyShiftTab:
		m.activeTab = (m.activeTab - 1 + activeTab(len(tabNames))) % activeTab(len(tabNames))
		m.telemetry.markTab(m.activeTab)
		return m, nil
	}

	if m.activeTab == tabConfig {
		return m.updateConfigTab(msg)
	}

	return m.updateChildren(msg)
}

func (m Model) updateConfigTab(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if keyString(msg) == keyEnter {
		if m.configTab.cursor == fieldSave && m.configTab.modified {
			m.configTab.ApplyToConfig(m.config)
			if err := shop.WriteConfig(m.config, m.projectRoot); err != nil {
				m.configTab.err = err
				m.configTab.saved = false
				return m, nil
			}
			if localCfg := m.configTab.LocalConfig(); localCfg != nil {
				if err := shop.WriteLocalConfig(localCfg, m.projectRoot); err != nil {
					m.configTab.err = err
					m.configTab.saved = false
					return m, nil
				}
			}
			if envChanges := m.configTab.ChangedEnvValues(); len(envChanges) > 0 {
				if err := envfile.WriteValues(m.projectRoot, envChanges); err != nil {
					m.configTab.err = err
					m.configTab.saved = false
					return m, nil
				}
				m.configTab.MarkEnvValuesPersisted()
			}
			m.configTab.modified = false
			m.configTab.err = nil
			if m.dockerMode {
				m.configTab.saved = false
				m.configTab.restarting = true
				return m, m.restartContainersForConfig()
			}
			m.configTab.saved = true
			return m, nil
		}
		if picker := m.configTab.PickerForCursor(); picker != nil {
			m.modal = picker
			return m, textinput.Blink
		}
		return m, nil
	}

	newConfig, cmd := m.configTab.HandleKey(msg)
	m.configTab = newConfig
	return m, cmd
}

func (m Model) executeCommand(id string) (tea.Model, tea.Cmd) {
	switch id {
	case "open-shop", "open-admin":
		m.telemetry.countAction()
		trackEvent(tracking.EventDevAction, map[string]string{tracking.TagAction: id})
		if id == "open-shop" {
			return m, openInBrowser(m.overview.shopURL)
		}
		return m, openInBrowser(m.overview.adminURL)
	case "cache-clear":
		m.telemetry.beginTask(id)
		return m, m.runCacheClear()
	case "admin-build":
		m.telemetry.beginTask(id)
		return m, m.runAdminBuild()
	case "sf-build":
		m.telemetry.beginTask(id)
		return m, m.runStorefrontBuild()
	case "admin-watch-start":
		if !m.overview.adminWatchRunning && !m.overview.adminWatchStarting {
			m.overview.adminWatchStarting = true
			return m, m.overview.startAdminWatch()
		}
	case "admin-watch-stop":
		if m.overview.adminWatchRunning {
			m.overview.adminWatchRunning = false
			return m, m.stopWatcher(watcherAdmin)
		}
	case "sf-watch-start":
		return m.openSalesChannelPicker()
	case "sf-watch-stop":
		if m.overview.sfWatchRunning {
			m.overview.sfWatchRunning = false
			return m, m.stopWatcher(watcherStorefront)
		}
	case "tab-instance":
		m.activeTab = tabInstance
		m.telemetry.markTab(m.activeTab)
	case "tab-overview":
		m.activeTab = tabOverview
		m.telemetry.markTab(m.activeTab)
	case "tab-config":
		m.activeTab = tabConfig
		m.telemetry.markTab(m.activeTab)
	case "quit":
		if m.dockerMode {
			m.modal = newStopConfirm()
			return m, nil
		}
		m.shutdown()
		return m, tea.Quit
	}
	return m, nil
}

// openSalesChannelPicker opens the sales-channel picker modal so the user can
// resolve a storefront's theme/domain before the watcher starts. Used by both
// the command palette and the Overview tab's storefront activation.
func (m Model) openSalesChannelPicker() (tea.Model, tea.Cmd) {
	if m.overview.sfWatchRunning || m.overview.sfWatchStarting {
		return m, nil
	}
	picker := newSalesChannelPicker(m.executor)
	m.modal = picker
	return m, picker.Init()
}

func (m *Model) stopWatcher(name string) tea.Cmd {
	if tags, ok := m.telemetry.watcherEndTags(name, watcherEndUserStopped); ok {
		trackEvent(tracking.EventDevWatcher, tags)
	}
	m.instance.RemoveSource(name)

	h := m.watchers[name]
	delete(m.watchers, name)

	return func() tea.Msg {
		if h != nil {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer stopCancel()
			h.stop(stopCtx)
		}

		return watcherStoppedMsg{name: name}
	}
}

func (m Model) updateMigrationWizard(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	// Enter on the welcome screen's "Quit" button exits the wizard from inside
	// migrationWizard.update, so detect it here before the state advances.
	welcomeQuit := m.migrationWizard.step == migrationStepWelcome &&
		!m.migrationWizard.confirmYes && keyString(msg) == keyEnter

	newGuide, cmd := m.migrationWizard.update(msg)
	m.migrationWizard = newGuide

	// Ctrl+C on any step quits the app
	if welcomeQuit || keyString(msg) == keyCtrlC {
		// The done screen already sent a completed/failed event for this run.
		if m.migrationWizard.step != migrationStepDone {
			trackEventNow(tracking.EventDevMigrationWizard, migrationWizardTags(tracking.ResultCancelled, m.migrationWizard))
		}
		return m, tea.Quit
	}

	// User pressed Enter on the review step. confirmYes=true saves and
	// continues, confirmYes=false picks the Quit button and exits the wizard.
	if m.migrationWizard.step == migrationStepReview && keyString(msg) == keyEnter {
		if m.migrationWizard.confirmYes {
			return m.saveMigrationWizard()
		}
		trackEventNow(tracking.EventDevMigrationWizard, migrationWizardTags(tracking.ResultCancelled, m.migrationWizard))
		return m, tea.Quit
	}

	// User pressed Enter on the done screen → start docker containers.
	// If the previous save errored, stay on the done screen so the user can read it.
	if m.migrationWizard.step == migrationStepDone && keyString(msg) == keyEnter && m.migrationWizard.err == nil {
		return m.startAfterMigrationWizard()
	}

	return m, cmd
}

func (m Model) saveMigrationWizard() (tea.Model, tea.Cmd) {
	m.migrationWizard.applyToConfig(m.config)
	if err := shop.WriteConfig(m.config, m.projectRoot); err != nil {
		m.migrationWizard.err = err
		m.migrationWizard.step = migrationStepDone
		trackEvent(tracking.EventDevMigrationWizard, migrationWizardTags(tracking.ResultFailed, m.migrationWizard))
		return m, nil
	}

	changed, err := ensureDeploymentHelper(m.projectRoot)
	if err != nil {
		m.migrationWizard.err = err
		m.migrationWizard.step = migrationStepDone
		trackEvent(tracking.EventDevMigrationWizard, migrationWizardTags(tracking.ResultFailed, m.migrationWizard))
		return m, nil
	}
	m.migrationWizard.deploymentHelperAdded = changed

	m.migrationWizard.step = migrationStepDone
	trackEvent(tracking.EventDevMigrationWizard, migrationWizardTags(tracking.ResultCompleted, m.migrationWizard))
	return m, nil
}

// mergeLocalProfilerSecrets copies profiler credential fields from the
// .shopware-project.local.yml partial config onto the main runtime config.
// Only profiler secrets are merged — other fields are intentionally left as
// the project-level config defines them.
func mergeLocalProfilerSecrets(dst, src *shop.Config) {
	if src == nil || src.Docker == nil || src.Docker.PHP == nil {
		return
	}
	if dst.Docker == nil {
		dst.Docker = &shop.ConfigDocker{}
	}
	if dst.Docker.PHP == nil {
		dst.Docker.PHP = &shop.ConfigDockerPHP{}
	}
	if v := src.Docker.PHP.BlackfireServerID; v != "" {
		dst.Docker.PHP.BlackfireServerID = v
	}
	if v := src.Docker.PHP.BlackfireServerToken; v != "" {
		dst.Docker.PHP.BlackfireServerToken = v
	}
	if v := src.Docker.PHP.TidewaysAPIKey; v != "" {
		dst.Docker.PHP.TidewaysAPIKey = v
	}
}

func (m Model) startAfterMigrationWizard() (tea.Model, tea.Cmd) {
	envCfg, err := m.config.ResolveEnvironment("")
	if err != nil {
		m.migrationWizard.err = err
		return m, nil
	}
	m.envConfig = envCfg

	exec, err := executor.New(m.projectRoot, envCfg, m.config)
	if err != nil {
		m.migrationWizard.err = err
		return m, nil
	}
	m.executor = exec

	if m.executor.Type() == executor.TypeDocker {
		if err := dockerpkg.WriteComposeFile(m.projectRoot, dockerpkg.ComposeOptionsFromConfig(m.config)); err != nil {
			m.migrationWizard.err = err
			return m, nil
		}
	}

	m.phase = phaseStarting
	m.overlayLines = nil
	m.dockerShowLogs = false
	m.dockerSpinner = newBrandSpinner()

	m.rebuildTabs()

	return m, tea.Batch(m.dockerSpinner.Tick, m.startContainers())
}

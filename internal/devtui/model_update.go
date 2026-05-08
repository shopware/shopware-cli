package devtui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/shop"
)

func (m Model) updateKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.modal != nil {
		next, cmd := m.modal.Update(msg)
		m.modal = next
		return m, cmd
	}

	if m.phase == phaseInstallPrompt {
		return m.updateInstallPrompt(msg)
	}

	if m.phase == phaseStarting || m.phase == phaseStopping {
		switch msg.String() {
		case "l":
			m.dockerShowLogs = !m.dockerShowLogs
		case keyQ, keyCtrlC:
			return m, tea.Quit
		}
		return m, nil
	}

	if m.phase == phaseInstalling {
		switch msg.String() {
		case "l":
			m.installProg.showLogs = !m.installProg.showLogs
		case keyQ, keyCtrlC:
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
		if msg.String() == keyQ || msg.String() == keyCtrlC {
			return m, tea.Quit
		}
		return m, nil
	}

	return m.updateDashboardKeys(msg)
}

func (m Model) updateDashboardKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+p":
		m.modal = newCommandPalette()
		return m, textinput.Blink
	case keyCtrlC, keyQ:
		if m.dockerMode {
			m.modal = newStopConfirm()
			return m, nil
		}
		m.shutdown()
		return m, tea.Quit
	case key1:
		m.activeTab = tabGeneral
		return m, nil
	case key2:
		m.activeTab = tabLogs
		return m, nil
	case key3:
		m.activeTab = tabConfig
		return m, nil
	case keyTab:
		m.activeTab = (m.activeTab + 1) % activeTab(len(tabNames))
		return m, nil
	case keyShiftTab:
		m.activeTab = (m.activeTab - 1 + activeTab(len(tabNames))) % activeTab(len(tabNames))
		return m, nil
	}

	if m.activeTab == tabConfig {
		return m.updateConfigTab(msg)
	}

	return m.updateChildren(msg)
}

func (m Model) updateConfigTab(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == keyEnter {
		if m.configTab.cursor == fieldSave && m.configTab.modified {
			m.configTab.ApplyToConfig(m.config)
			_ = shop.WriteConfig(m.config, m.projectRoot)
			if localCfg := m.configTab.LocalConfig(); localCfg != nil {
				_ = shop.WriteLocalConfig(localCfg, m.projectRoot)
			}
			return m, func() tea.Msg { return configSavedMsg{} }
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
	case "open-shop":
		return m, openInBrowser(m.general.shopURL)
	case "open-admin":
		return m, openInBrowser(m.general.adminURL)
	case "cache-clear":
		return m, m.runCacheClear()
	case "admin-build":
		return m, m.runAdminBuild()
	case "sf-build":
		return m, m.runStorefrontBuild()
	case "admin-watch-start":
		if !m.general.adminWatchRunning && !m.general.adminWatchStarting {
			m.general.adminWatchStarting = true
			return m, m.general.startAdminWatch()
		}
	case "admin-watch-stop":
		if m.general.adminWatchRunning {
			m.general.adminWatchRunning = false
			return m, m.stopWatcher(watcherAdmin)
		}
	case "sf-watch-start":
		if !m.general.sfWatchRunning && !m.general.sfWatchStarting {
			m.general.sfWatchStarting = true
			return m, m.general.startStorefrontWatch()
		}
	case "sf-watch-stop":
		if m.general.sfWatchRunning {
			m.general.sfWatchRunning = false
			return m, m.stopWatcher(watcherStorefront)
		}
	case "tab-logs":
		m.activeTab = tabLogs
	case "tab-general":
		m.activeTab = tabGeneral
	case "tab-config":
		m.activeTab = tabConfig
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

func (m *Model) stopWatcher(name string) tea.Cmd {
	// Stop log streaming first.
	m.logs.StopStreaming()

	// Get and remove the tracked process.
	p := m.watchers[name]
	delete(m.watchers, name)

	return func() tea.Msg {
		if p != nil {
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer stopCancel()
			_ = p.Stop(stopCtx)
		}

		return watcherStoppedMsg{name: name}
	}
}

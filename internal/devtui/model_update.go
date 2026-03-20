package devtui

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/shop"
)

func (m Model) updateLifecycle(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dockerAlreadyRunningMsg:
		m.overlay = overlayNone
		return m, m.checkShopwareInstalled()

	case dockerNeedStartMsg:
		m.overlay = overlayStarting
		m.overlayLines = nil
		m.dockerShowLogs = false
		m.dockerSpinner = newBrandSpinner()
		return m, tea.Batch(m.dockerSpinner.Tick, m.startContainers())

	case dockerOutputLineMsg:
		m.overlayLines = append(m.overlayLines, string(msg))
		maxLines := m.overlayMaxLines()
		if len(m.overlayLines) > maxLines {
			m.overlayLines = m.overlayLines[len(m.overlayLines)-maxLines:]
		}
		if m.overlay == overlayInstalling {
			line := string(msg)
			if strings.HasPrefix(line, "Start: ") {
				for i, sp := range installStepPatterns {
					if strings.Contains(line, sp.pattern) && i >= m.installProg.currentStep {
						m.installProg.currentStep = i
						pct := float64(i) / float64(len(installStepPatterns))
						cmd := m.installProg.progress.SetPercent(pct)
						return m, tea.Batch(cmd, m.readNextDockerOutput())
					}
				}
			}
		}
		return m, m.readNextDockerOutput()

	case dockerOutputDoneMsg:
		return m, nil

	case dockerStartedMsg:
		if msg.err != nil {
			m.overlayLines = append(m.overlayLines, errorStyle.Render("Failed: "+msg.err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}
		m.overlay = overlayNone
		m.overlayLines = nil
		m.dockerOutChan = nil
		return m, m.checkShopwareInstalled()

	case shopwareInstalledMsg:
		m.overlay = overlayNone
		return m, m.startDashboard()

	case shopwareNotInstalledMsg:
		m.overlay = overlayInstallPrompt
		m.overlayLines = nil

		usernameInput := textinput.New()
		usernameInput.Placeholder = defaultUsername
		usernameInput.Prompt = "Username: "
		usernameInput.CharLimit = 50

		passwordInput := textinput.New()
		passwordInput.Placeholder = "shopware"
		passwordInput.Prompt = "Password: "
		passwordInput.CharLimit = 50

		m.install = installWizard{step: installStepAsk, confirmYes: true, username: usernameInput, password: passwordInput}
		return m, nil

	case shopwareInstallDoneMsg:
		if msg.err != nil {
			m.installProg.showLogs = true
			m.overlayLines = append(m.overlayLines, "", errorStyle.Render("Installation failed: "+msg.err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}
		m.installProg.done = true
		m.installProg.currentStep = len(installStepPatterns)

		username := m.install.username.Value()
		password := m.install.password.Value()

		adminApi := &shop.ConfigAdminApi{
			Username: username,
			Password: password,
		}
		m.envConfig.AdminApi = adminApi
		_ = shop.WriteConfig(m.config, m.projectRoot)

		m.general.username = username
		m.general.password = password

		m.overlay = overlayNone
		m.overlayLines = nil
		m.dockerOutChan = nil
		return m, m.startDashboard()

	case dockerStoppedMsg:
		return m, tea.Quit
	}

	return m, nil
}

func (m Model) updateKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.overlay == overlayCommandPalette {
		return m.updateCommandPalette(msg)
	}

	if m.overlay == overlayInstallPrompt {
		return m.updateInstallPrompt(msg)
	}

	if m.overlay == overlayStopConfirm {
		switch msg.String() {
		case "left", "h":
			m.stopConfirmYes = true
		case "right", "l":
			m.stopConfirmYes = false
		case keyTab:
			m.stopConfirmYes = !m.stopConfirmYes
		case keyEnter:
			if m.stopConfirmYes {
				m.overlay = overlayStopping
				m.overlayLines = nil
				m.dockerShowLogs = false
				m.dockerSpinner = newBrandSpinner()
				return m, tea.Batch(m.dockerSpinner.Tick, m.stopContainers())
			}
			return m, tea.Quit
		}
		return m, nil
	}

	if m.overlay == overlayStarting || m.overlay == overlayStopping {
		switch msg.String() {
		case "l":
			m.dockerShowLogs = !m.dockerShowLogs
		case keyQ, keyCtrlC:
			return m, tea.Quit
		}
		return m, nil
	}

	if m.overlay == overlayInstalling {
		switch msg.String() {
		case "l":
			m.installProg.showLogs = !m.installProg.showLogs
		case keyQ, keyCtrlC:
			return m, tea.Quit
		}
		return m, nil
	}

	if m.overlay != overlayNone {
		if msg.String() == keyQ || msg.String() == keyCtrlC {
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+p":
		m.overlay = overlayCommandPalette
		m.palette = newCommandPalette()
		return m, textinput.Blink
	case keyCtrlC, keyQ:
		m.logs.StopStreaming()
		if m.dockerMode {
			m.overlay = overlayStopConfirm
			m.overlayLines = nil
			m.stopConfirmYes = true
			return m, nil
		}
		return m, tea.Quit
	case key1:
		m.activeTab = tabGeneral
		return m, nil
	case key2:
		m.activeTab = tabLogs
		return m, nil
	case keyTab, keyShiftTab:
		m.activeTab = (m.activeTab + 1) % 2
		return m, nil
	}

	return m.updateChildren(msg)
}

func (m Model) updateCommandPalette(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+p":
		m.overlay = overlayNone
		return m, nil
	case keyUp, keyK:
		m.palette.moveUp()
		return m, nil
	case keyDown, keyJ:
		m.palette.moveDown()
		return m, nil
	case keyEnter:
		id := m.palette.selectedID()
		m.overlay = overlayNone
		return m.executeCommand(id)
	}

	var cmd tea.Cmd
	m.palette.filter, cmd = m.palette.filter.Update(msg)
	m.palette.applyFilter()
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
	case "tab-logs":
		m.activeTab = tabLogs
	case "tab-general":
		m.activeTab = tabGeneral
	case "quit":
		m.logs.StopStreaming()
		if m.dockerMode {
			m.overlay = overlayStopConfirm
			m.stopConfirmYes = true
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) runCacheClear() tea.Cmd {
	e := m.executor
	return func() tea.Msg {
		cmd := e.ConsoleCommand(context.Background(), "cache:clear")
		_ = cmd.Run()
		return nil
	}
}

func (m Model) updateInstallPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyQ, keyCtrlC:
		return m, tea.Quit
	}

	switch m.install.step {
	case installStepAsk:
		switch msg.String() {
		case "left", "h":
			m.install.confirmYes = true
		case "right", "l":
			m.install.confirmYes = false
		case keyTab:
			m.install.confirmYes = !m.install.confirmYes
		case keyEnter:
			if m.install.confirmYes {
				m.install.step = installStepLanguage
				m.install.cursor = 0
			} else {
				m.overlay = overlayNone
				return m, m.startDashboard()
			}
		}

	case installStepLanguage:
		switch msg.String() {
		case keyUp, keyK:
			if m.install.cursor > 0 {
				m.install.cursor--
			}
		case keyDown, keyJ:
			if m.install.cursor < len(installLanguages)-1 {
				m.install.cursor++
			}
		case keyEnter:
			m.install.language = installLanguages[m.install.cursor].id
			m.install.step = installStepCurrency
			m.install.cursor = 0
		}

	case installStepCurrency:
		switch msg.String() {
		case keyUp, keyK:
			if m.install.cursor > 0 {
				m.install.cursor--
			}
		case keyDown, keyJ:
			if m.install.cursor < len(installCurrencies)-1 {
				m.install.cursor++
			}
		case keyEnter:
			m.install.currency = installCurrencies[m.install.cursor]
			m.install.step = installStepUsername
			m.install.username.SetValue(defaultUsername)
			m.install.username.Focus()
			return m, textinput.Blink
		}

	case installStepUsername:
		switch msg.String() {
		case keyEnter:
			m.install.step = installStepPassword
			m.install.username.Blur()
			m.install.password.SetValue("shopware")
			m.install.password.Focus()
			return m, textinput.Blink
		default:
			var cmd tea.Cmd
			m.install.username, cmd = m.install.username.Update(msg)
			return m, cmd
		}

	case installStepPassword:
		switch msg.String() {
		case keyEnter:
			m.install.password.Blur()
			m.overlay = overlayInstalling
			m.overlayLines = nil
			m.installProg = installProgress{
				spinner:  newBrandSpinner(),
				progress: newInstallProgress(),
			}
			return m, tea.Batch(m.installProg.spinner.Tick, m.runShopwareInstall())
		default:
			var cmd tea.Cmd
			m.install.password, cmd = m.install.password.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

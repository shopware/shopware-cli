package devtui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/shop"
)

func (m Model) updateLifecycle(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dockerAlreadyRunningMsg:
		m.phase = phaseDashboard
		return m, m.checkShopwareInstalled()

	case dockerNeedStartMsg:
		m.phase = phaseStarting
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
		if m.phase == phaseInstalling {
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
		if tags, ok := m.telemetry.dockerStartTags(msg.err); ok {
			trackEvent(eventDevDockerStart, tags)
		}
		if msg.err != nil {
			m.dockerShowLogs = true
			m.overlayLines = append(m.overlayLines, errorStyle.Render("Failed: "+msg.err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}
		m.phase = phaseDashboard
		m.overlayLines = nil
		m.dockerOutChan = nil
		return m, m.checkShopwareInstalled()

	case shopwareInstalledMsg:
		m.phase = phaseDashboard
		return m, m.startDashboard()

	case shopwareNotInstalledMsg:
		m.phase = phaseInstallPrompt
		m.overlayLines = nil

		m.install = installWizard{
			credentialStep: newInstallCredentialStep(),
			step:           installStepAsk,
			confirmYes:     true,
		}
		return m, nil

	case shopwareInstallDoneMsg:
		if msg.err != nil {
			if m.telemetry.installOnce() {
				tags := m.telemetry.installTags("failure", m.install)
				tags["failed_step"] = installFailedStep(m.installProg.currentStep)
				trackEvent(eventDevInstall, tags)
			}
			m.installProg.showLogs = true
			m.overlayLines = append(m.overlayLines, "", errorStyle.Render("Installation failed: "+msg.err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}
		m.installProg.done = true
		m.installProg.currentStep = len(installStepPatterns)
		if m.telemetry.installOnce() {
			trackEvent(eventDevInstall, m.telemetry.installTags("success", m.install))
		}

		username := m.install.username.Value()
		password := m.install.password.Value()

		adminApi := &shop.ConfigAdminApi{
			Username: username,
			Password: password,
		}
		m.envConfig.AdminApi = adminApi
		_ = shop.WriteConfig(m.config, m.projectRoot)

		m.overview.username = username
		m.overview.password = password

		m.phase = phaseDashboard
		m.overlayLines = nil
		m.dockerOutChan = nil
		return m, m.startDashboard()

	case dockerStoppedMsg:
		return m, tea.Quit
	}

	return m, nil
}

package dev

import (
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/shop/install"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

func (m Model) updateLifecycle(msg tea.Msg) (app.Content, tea.Cmd) {
	switch msg := msg.(type) {
	case dockerAlreadyRunningMsg:
		m.phase = phaseDashboard
		return m, m.checkShopwareInstalled()

	case dockerNeedStartMsg:
		m.phase = phaseStarting
		m.overlayLines = nil
		m.dockerShowLogs = false
		m.dockerSpinner = tui.NewBrandSpinner()
		return m, tea.Batch(m.dockerSpinner.Tick, m.startContainers())

	case dockerOutputLineMsg:
		m.overlayLines = tui.AppendTail(m.overlayLines, m.overlayMaxLines(), string(msg))
		if m.phase == phaseInstalling {
			if i, ok := install.MatchStep(string(msg), m.installProg.currentStep); ok {
				m.installProg.currentStep = i
				pct := float64(i) / float64(len(install.Steps))
				cmd := m.installProg.progress.SetPercent(pct)
				return m, tea.Batch(cmd, m.readNextDockerOutput())
			}
		}
		return m, m.readNextDockerOutput()

	case dockerOutputDoneMsg:
		return m, nil

	case dockerStartedMsg:
		if tags, ok := m.telemetry.dockerStartTags(msg.err); ok {
			trackEvent(tracking.EventDevDockerStart, tags)
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
			CredentialStep: newInstallCredentialStep(),
			step:           installStepAsk,
			confirmYes:     true,
		}
		return m, nil

	case shopwareInstallDoneMsg:
		if msg.err != nil {
			failure := classifyInstallFailure(msg.output)
			if m.telemetry.installOnce() {
				trackEvent(tracking.EventDevInstall, m.telemetry.installFailureTags(m.install, failure))
			}
			m.installProg.showLogs = true
			m.overlayLines = append(m.overlayLines, "", errorStyle.Render("Installation failed: "+msg.err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}
		m.installProg.done = true
		m.installProg.currentStep = len(install.Steps)

		username := m.install.Username()
		password := m.install.Password()

		// Persist before recording the outcome, so a failed config write is
		// not reported as a successful run.
		if err := install.PersistCredentials(m.config, m.envConfig, m.projectRoot, install.Options{
			AdminUsername: username,
			AdminPassword: password,
		}); err != nil {
			if m.telemetry.installOnce() {
				tags := m.telemetry.installTags(tracking.ResultFailure, m.install)
				tags[tracking.TagFailedStep] = install.FailedStepSaveCredentials
				trackEvent(tracking.EventDevInstall, tags)
			}
			m.installProg.showLogs = true
			m.overlayLines = append(m.overlayLines, "", errorStyle.Render("Shopware was installed, but saving the admin credentials to the project config failed: "+err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}

		if m.telemetry.installOnce() {
			trackEvent(tracking.EventDevInstall, m.telemetry.installTags(tracking.ResultSuccess, m.install))
		}

		m.overview.username = username
		m.overview.password = password

		m.phase = phaseDashboard
		m.overlayLines = nil
		m.dockerOutChan = nil
		return m, m.startDashboard()

	case dockerStoppedMsg:
		return m, tea.Quit

	case portConflictMsg:
		m.phase = phasePortConflict
		m.portConflicts = msg.conflicts
		return m, m.host.PushOverlay(newPortConflictPrompt(msg.conflicts))

	case portFixDoneMsg:
		if msg.err != nil {
			m.dockerShowLogs = true
			m.overlayLines = append(m.overlayLines, errorStyle.Render("Failed: "+msg.err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}
		// The command goroutine built a detached copy; adopt it here on the
		// update thread. The tabs share the config pointer, so copy into it
		// rather than swapping the pointer.
		*m.config = *msg.config
		m.overview.setEnvironment(m.dockerEnvironment())
		return m, m.startContainers()
	}

	return m, nil
}

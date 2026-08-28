package dev

import (
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/shop/install"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

type installStep int

const (
	installStepAsk installStep = iota
	installStepLanguage
	installStepCurrency
	installStepCredentials
)

type installWizard struct {
	tui.CredentialStep

	step       installStep
	cursor     int
	confirmYes bool
	language   string
	currency   string
}

// newInstallCredentialStep builds the install wizard's credential inputs. They
// start empty (filled in later from the chosen defaults) and use labelled
// prompts that match the install prompt layout.
func newInstallCredentialStep() tui.CredentialStep {
	return tui.NewCredentialStep(tui.CredentialStepOptions{
		UsernamePlaceholder: install.DefaultAdminUsername,
		UsernamePrompt:      "Username: ",
		PasswordPlaceholder: install.DefaultAdminPassword,
		PasswordPrompt:      "Password: ",
		CharLimit:           50,
		ValidatePassword:    install.ValidateAdminPassword,
	})
}

type installProgress struct {
	currentStep int
	done        bool
	showLogs    bool
	spinner     spinner.Model
	progress    progress.Model
}

func (m Model) updateInstallPrompt(msg tea.KeyPressMsg) (app.Content, tea.Cmd) {
	if k := tui.KeyString(msg); k == "q" || k == tui.KeyCtrlC {
		if m.telemetry.installOnce() {
			tags := m.telemetry.installTags(tracking.ResultCancelled, m.install)
			tags[tracking.TagAbandonedAt] = installStepTagName(m.install.step)
			trackEventNow(tracking.EventDevInstall, tags)
		}
		return m, tea.Quit
	}

	switch m.install.step {
	case installStepAsk:
		return m.updateInstallStepAsk(msg)
	case installStepLanguage:
		return m.updateInstallStepLanguage(msg)
	case installStepCurrency:
		return m.updateInstallStepCurrency(msg)
	case installStepCredentials:
		return m.updateInstallStepCredentials(msg)
	}

	return m, nil
}

func (m Model) updateInstallStepAsk(msg tea.KeyPressMsg) (app.Content, tea.Cmd) {
	key := tui.KeyString(msg)
	m.install.confirmYes = tui.ConfirmNav(m.install.confirmYes, key)
	if key == tui.KeyEnter {
		if m.install.confirmYes {
			m.install.step = installStepLanguage
			m.install.cursor = 0
			return m, nil
		}
		if m.telemetry.installOnce() {
			trackEvent(tracking.EventDevInstall, m.telemetry.installTags(tracking.ResultSkipped, m.install))
		}
		m.phase = phaseDashboard
		return m, m.startDashboard()
	}
	return m, nil
}

func (m Model) updateInstallStepLanguage(msg tea.KeyPressMsg) (app.Content, tea.Cmd) {
	if tui.KeyString(msg) == tui.KeyEnter {
		m.install.language = install.Languages[m.install.cursor].ID
		m.install.step = installStepCurrency
		m.install.cursor = 0
		return m, nil
	}
	m.install.cursor = tui.MoveCursor(m.install.cursor, tui.KeyString(msg), len(install.Languages))
	return m, nil
}

func (m Model) updateInstallStepCurrency(msg tea.KeyPressMsg) (app.Content, tea.Cmd) {
	if tui.KeyString(msg) == tui.KeyEnter {
		m.install.currency = install.Currencies[m.install.cursor]
		m.install.step = installStepCredentials
		m.install.SetUsername(install.DefaultAdminUsername)
		m.install.SetPassword(install.DefaultAdminPassword)
		return m, m.install.Focus(tui.CredFocusUsername)
	}
	m.install.cursor = tui.MoveCursor(m.install.cursor, tui.KeyString(msg), len(install.Currencies))
	return m, nil
}

func (m Model) updateInstallStepCredentials(msg tea.KeyPressMsg) (app.Content, tea.Cmd) {
	cmd, submitted := m.install.HandleKey(msg)
	if !submitted {
		return m, cmd
	}
	m.telemetry.beginInstall()
	m.phase = phaseInstalling
	m.overlayLines = nil
	m.installProg = installProgress{
		spinner:  tui.NewBrandSpinner(),
		progress: newInstallProgress(),
	}
	return m, tea.Batch(m.installProg.spinner.Tick, m.runShopwareInstall())
}

func (m Model) renderInstallPrompt(b *strings.Builder) {
	switch m.install.step {
	case installStepAsk:
		warnStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor)
		b.WriteString(warnStyle.Render("Shopware is not initialized yet"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("This project has not been set up yet. The installation\nwill create the database, run migrations and configure\nyour local development environment."))
		b.WriteString("\n\n")
		b.WriteString(tui.ConfirmButtons("Initialize now", "No, skip", m.install.confirmYes))

	case installStepLanguage:
		b.WriteString(tui.TextBadge("Step 1/3"))
		b.WriteString("\n\n")
		opts := make([]tui.SelectOption, len(install.Languages))
		for i, lang := range install.Languages {
			opts[i] = tui.SelectOption{Label: lang.Label, Detail: lang.ID}
		}
		b.WriteString(tui.RenderSelectList("Default Language", "Select the primary language for your storefront", opts, m.install.cursor))

	case installStepCurrency:
		b.WriteString(tui.TextBadge("Step 2/3"))
		b.WriteString("\n\n")
		opts := make([]tui.SelectOption, len(install.Currencies))
		for i, curr := range install.Currencies {
			opts[i] = tui.SelectOption{Label: curr}
		}
		b.WriteString(tui.RenderSelectList("Default Currency", "Select the default currency for pricing", opts, m.install.cursor))

	case installStepCredentials:
		b.WriteString(tui.TextBadge("Step 3/3"))
		b.WriteString("\n\n")
		b.WriteString(tui.TitleStyle.Render("Admin Account"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("The login for the Shopware admin panel and API."))
		b.WriteString("\n\n")
		m.install.Render(b)
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("Used to create the Shopware admin user."))
	}
}

func (m Model) installFooterHint() string {
	switch m.install.step {
	case installStepAsk:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case installStepLanguage, installStepCurrency:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "↑/↓", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case installStepCredentials:
		return m.install.FooterHint("Install")
	}
	return ""
}

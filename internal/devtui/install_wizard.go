package devtui

import (
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

type installStep int

const (
	installStepAsk installStep = iota
	installStepLanguage
	installStepCurrency
	installStepUsername
	installStepPassword
)

type installLanguage struct {
	id    string
	label string
}

var (
	installLanguages = []installLanguage{
		{"en-GB", "English (UK)"},
		{"en-US", "English (US)"},
		{"de-DE", "Deutsch"},
		{"cs-CZ", "Čeština"},
		{"da-DK", "Dansk"},
		{"es-ES", "Español"},
		{"fr-FR", "Français"},
		{"it-IT", "Italiano"},
		{"nl-NL", "Nederlands"},
		{"nn-NO", "Norsk"},
		{"pl-PL", "Język polski"},
		{"pt-PT", "Português"},
		{"sv-SE", "Svenska"},
	}
	installCurrencies = []string{"EUR", "USD", "GBP", "PLN", "CHF", "SEK", "DKK", "NOK", "CZK"}
)

type installWizard struct {
	step            installStep
	cursor          int
	confirmYes      bool
	language        string
	currency        string
	username        textinput.Model
	password        textinput.Model
	checkboxFocused bool
}

type installProgress struct {
	currentStep int
	done        bool
	showLogs    bool
	spinner     spinner.Model
	progress    progress.Model
}

var installStepPatterns = []struct {
	pattern string
	label   string
}{
	{"system:install", "Installing Shopware"},
	{"user:create", "Creating admin account"},
	{"messenger:setup-transports", "Setting up message transports"},
	{"sales-channel:create:storefront", "Creating storefront"},
	{"theme:change", "Compiling theme"},
	{"plugin:refresh", "Refreshing plugins"},
}

func (m Model) updateInstallPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if k := msg.String(); k == keyQ || k == keyCtrlC {
		return m, tea.Quit
	}

	switch m.install.step {
	case installStepAsk:
		return m.updateInstallStepAsk(msg)
	case installStepLanguage:
		return m.updateInstallStepLanguage(msg)
	case installStepCurrency:
		return m.updateInstallStepCurrency(msg)
	case installStepUsername:
		return m.updateInstallStepUsername(msg)
	case installStepPassword:
		return m.updateInstallStepPassword(msg)
	}

	return m, nil
}

func (m Model) updateInstallStepAsk(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyLeft, "h":
		m.install.confirmYes = true
	case keyRight, "l":
		m.install.confirmYes = false
	case keyTab:
		m.install.confirmYes = !m.install.confirmYes
	case keyEnter:
		if m.install.confirmYes {
			m.install.step = installStepLanguage
			m.install.cursor = 0
			return m, nil
		}
		m.phase = phaseDashboard
		return m, m.startDashboard()
	}
	return m, nil
}

func (m Model) updateInstallStepLanguage(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	return m, nil
}

func (m Model) updateInstallStepCurrency(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
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
	return m, nil
}

func (m Model) updateInstallStepUsername(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == keyEnter {
		m.install.step = installStepPassword
		m.install.username.Blur()
		m.install.password.SetValue("shopware")
		m.install.password.Focus()
		m.install.checkboxFocused = false
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.install.username, cmd = m.install.username.Update(msg)
	return m, cmd
}

func (m Model) updateInstallStepPassword(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyEnter:
		return m.handleInstallPasswordEnter()
	case keyTab, keyDown:
		if !m.install.checkboxFocused {
			m.install.checkboxFocused = true
			m.install.password.Blur()
		}
		return m, nil
	case keyShiftTab, keyUp:
		if m.install.checkboxFocused {
			m.install.checkboxFocused = false
			m.install.password.Focus()
			return m, textinput.Blink
		}
		return m, nil
	}
	if m.install.checkboxFocused {
		return m, nil
	}
	var cmd tea.Cmd
	m.install.password, cmd = m.install.password.Update(msg)
	return m, cmd
}

func (m Model) handleInstallPasswordEnter() (tea.Model, tea.Cmd) {
	if m.install.checkboxFocused {
		if m.install.password.EchoMode == textinput.EchoPassword {
			m.install.password.EchoMode = textinput.EchoNormal
		} else {
			m.install.password.EchoMode = textinput.EchoPassword
		}
		return m, nil
	}
	m.install.password.Blur()
	m.phase = phaseInstalling
	m.overlayLines = nil
	m.installProg = installProgress{
		spinner:  newBrandSpinner(),
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
		b.WriteString(renderConfirmButtons("Initialize now", "No, skip", m.install.confirmYes))

	case installStepLanguage:
		b.WriteString(tui.TextBadge("Step 1/4"))
		b.WriteString("\n\n")
		opts := make([]tui.SelectOption, len(installLanguages))
		for i, lang := range installLanguages {
			opts[i] = tui.SelectOption{Label: lang.label, Detail: lang.id}
		}
		b.WriteString(tui.RenderSelectList("Default Language", "Select the primary language for your storefront", opts, m.install.cursor))

	case installStepCurrency:
		b.WriteString(tui.TextBadge("Step 2/4"))
		b.WriteString("\n\n")
		opts := make([]tui.SelectOption, len(installCurrencies))
		for i, curr := range installCurrencies {
			opts[i] = tui.SelectOption{Label: curr}
		}
		b.WriteString(tui.RenderSelectList("Default Currency", "Select the default currency for pricing", opts, m.install.cursor))

	case installStepUsername:
		b.WriteString(tui.TextBadge("Step 3/4"))
		b.WriteString("\n\n")
		b.WriteString(tui.TitleStyle.Render("Admin Username"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Enter the username for the admin account"))
		b.WriteString("\n\n")
		b.WriteString(m.install.username.View())

	case installStepPassword:
		b.WriteString(tui.TextBadge("Step 4/4"))
		b.WriteString("\n\n")
		b.WriteString(tui.TitleStyle.Render("Admin Password"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Enter the password for the admin account (default: shopware)"))
		b.WriteString("\n\n")
		b.WriteString(m.install.password.View())
		b.WriteString("\n\n")
		b.WriteString(renderShowPasswordCheckbox(m.install.password.EchoMode == textinput.EchoNormal, m.install.checkboxFocused))
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
	case installStepUsername:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "enter", Label: "Continue"},
		)
	case installStepPassword:
		if m.install.checkboxFocused {
			return tui.ShortcutBar(
				tui.Shortcut{Key: "↑", Label: "Back"},
				tui.Shortcut{Key: "enter", Label: "Toggle"},
			)
		}
		return tui.ShortcutBar(
			tui.Shortcut{Key: "↓/tab", Label: "Show password"},
			tui.Shortcut{Key: "enter", Label: "Install"},
		)
	}
	return ""
}

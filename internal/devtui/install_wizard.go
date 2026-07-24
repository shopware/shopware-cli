package devtui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// minAdminPasswordLength is the minimum admin password length enforced by the
// Shopware core (user:create / system:install). Validating it here lets the
// wizard reject too-short passwords up front instead of failing late during the
// deployment-helper run.
const minAdminPasswordLength = 8

// validateAdminPassword mirrors the Shopware core password length requirement.
func validateAdminPassword(password string) error {
	if len([]rune(password)) < minAdminPasswordLength {
		return fmt.Errorf("password must be at least %d characters long", minAdminPasswordLength)
	}
	return nil
}

type installStep int

const (
	installStepAsk installStep = iota
	installStepLanguage
	installStepCurrency
	installStepCredentials
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
		UsernamePlaceholder: defaultUsername,
		UsernamePrompt:      "Username: ",
		PasswordPlaceholder: "shopware",
		PasswordPrompt:      "Password: ",
		CharLimit:           50,
		ValidatePassword:    validateAdminPassword,
	})
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
		m.install.language = installLanguages[m.install.cursor].id
		m.install.step = installStepCurrency
		m.install.cursor = 0
		return m, nil
	}
	m.install.cursor = tui.MoveCursor(m.install.cursor, tui.KeyString(msg), len(installLanguages))
	return m, nil
}

func (m Model) updateInstallStepCurrency(msg tea.KeyPressMsg) (app.Content, tea.Cmd) {
	if tui.KeyString(msg) == tui.KeyEnter {
		m.install.currency = installCurrencies[m.install.cursor]
		m.install.step = installStepCredentials
		m.install.SetUsername(defaultUsername)
		m.install.SetPassword("shopware")
		return m, m.install.Focus(tui.CredFocusUsername)
	}
	m.install.cursor = tui.MoveCursor(m.install.cursor, tui.KeyString(msg), len(installCurrencies))
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
		opts := make([]tui.SelectOption, len(installLanguages))
		for i, lang := range installLanguages {
			opts[i] = tui.SelectOption{Label: lang.label, Detail: lang.id}
		}
		b.WriteString(tui.RenderSelectList("Default Language", "Select the primary language for your storefront", opts, m.install.cursor))

	case installStepCurrency:
		b.WriteString(tui.TextBadge("Step 2/3"))
		b.WriteString("\n\n")
		opts := make([]tui.SelectOption, len(installCurrencies))
		for i, curr := range installCurrencies {
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

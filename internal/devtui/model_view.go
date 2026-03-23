package devtui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("")
	}

	v := tea.NewView("")
	v.AltScreen = true

	switch m.overlay {
	case overlayNone:
		v.Content = m.renderDashboard()
	case overlayCommandPalette:
		v.Content = m.palette.view(m.width, m.height)
	case overlayStarting, overlayStopConfirm, overlayStopping, overlayInstallPrompt, overlayInstalling:
		v.Content = m.renderOverlay()
	}

	return v
}

func (m Model) renderDashboard() string {
	tabHeader := buildTabHeader(int(m.activeTab), m.width)
	footer := m.renderDashboardFooter()

	footerHeight := lipgloss.Height(footer)
	boxHeight := m.height - 3 - footerHeight

	padV := 1
	padH := 3
	if m.activeTab == tabLogs {
		padV = 0
		padH = 1
	}

	contentH := boxHeight - padV*2 - 1
	contentW := m.width - padH*2 - 2

	var content string
	switch m.activeTab {
	case tabGeneral:
		content = m.general.View(m.width, boxHeight)
	case tabLogs:
		m.logs.SetSize(contentW, contentH)
		content = m.logs.View()
	}

	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderForeground(tui.BorderColor).
		Padding(padV, padH).
		Width(m.width).
		Height(boxHeight)

	return tabHeader + "\n" + contentBox.Render(content) + "\n" + footer
}

func (m Model) renderDashboardFooter() string {
	if m.activeTab == tabLogs {
		followState := "Follow"
		shortcuts := []tui.Shortcut{
			{Key: "↑/↓", Label: "Move cursor"},
			{Key: "enter", Label: "Open source"},
			{Key: "f", Label: followState},
			{Key: "tab", Label: "Next tab"},
			{Key: "ctrl+c", Label: "Exit"},
		}
		return tui.ShortcutBar(shortcuts...)
	}

	return tui.ShortcutBar(
		tui.Shortcut{Key: "ctrl+p", Label: "Commands"},
		tui.Shortcut{Key: "tab", Label: "Next tab"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	)
}

func (m Model) renderOverlay() string {
	var content strings.Builder
	var footerHint string

	switch m.overlay {
	case overlayStarting:
		footerHint = tui.ShortcutBadge("l", "Toggle logs")
		if m.dockerShowLogs {
			return m.renderDockerLogs("Starting Docker containers...", footerHint)
		}
		cardContent := fmt.Sprintf("%s Starting Docker containers...", m.dockerSpinner.View())
		content.WriteString(tui.RenderPhaseCard(cardContent))
	case overlayStopConfirm:
		var card strings.Builder
		warnStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor)
		card.WriteString(warnStyle.Render("Stop Docker containers?"))
		card.WriteString("\n")
		card.WriteString(tui.DimStyle.Render("Do you want to stop the running Docker containers?\nThey can be restarted with shopware-cli project dev."))
		card.WriteString("\n\n")
		card.WriteString(renderConfirmButtons("Yes, stop", "No, quit", m.stopConfirmYes))
		content.WriteString(tui.RenderPhaseCard(card.String()))
		footerHint = tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case overlayStopping:
		footerHint = tui.ShortcutBadge("l", "Toggle logs")
		if m.dockerShowLogs {
			return m.renderDockerLogs("Stopping Docker containers...", footerHint)
		}
		cardContent := fmt.Sprintf("%s Stopping Docker containers...", m.dockerSpinner.View())
		content.WriteString(tui.RenderPhaseCard(cardContent))
	case overlayInstallPrompt:
		var card strings.Builder
		m.renderInstallPrompt(&card)
		content.WriteString(tui.RenderPhaseCard(card.String()))
		footerHint = m.installFooterHint()
	case overlayInstalling:
		if m.installProg.showLogs {
			footerHint = tui.ShortcutBadge("l", "Toggle logs")
			return m.renderDockerLogs("Installing Shopware...", footerHint)
		}
		var card strings.Builder
		total := len(installStepPatterns)
		pctText := fmt.Sprintf(" %d%%", int(float64(m.installProg.currentStep)/float64(total)*100))
		card.WriteString(m.installProg.progress.View() + tui.DimStyle.Render(pctText) + "\n\n")

		for i, sp := range installStepPatterns {
			switch {
			case i < m.installProg.currentStep:
				card.WriteString(tui.StepDone(sp.label))
			case i == m.installProg.currentStep && !m.installProg.done:
				card.WriteString(tui.StepActive(m.installProg.spinner.View(), sp.label))
			case i == m.installProg.currentStep && m.installProg.done:
				card.WriteString(tui.StepDone(sp.label))
			default:
				card.WriteString(tui.StepPending(tui.DimStyle.Render(sp.label)))
			}
		}
		content.WriteString(tui.RenderPhaseCard(strings.TrimRight(card.String(), "\n")))
		footerHint = tui.ShortcutBadge("l", "Toggle logs")
	case overlayNone, overlayCommandPalette:
	}

	return renderPhaseLayout(content.String(), m.width, m.height, footerHint)
}

// phaseHeaderFooter builds the branding header and shortcut footer used by
// full-screen phase views, returning the rendered strings and the remaining
// box height.
func phaseHeaderFooter(width, height int, footerHint string) (header, footer string, boxHeight int) {
	branding := tui.BrandingLine()
	fill := width - tui.BrandingLineWidth()
	if fill < 0 {
		fill = 0
	}
	header = strings.Repeat(" ", fill) + branding

	exit := tui.ShortcutBadge("ctrl+c", "Exit")
	if footerHint != "" {
		sep := lipgloss.NewStyle().Foreground(tui.BorderColor).Render("  │  ")
		footer = footerHint + sep + exit
	} else {
		footer = exit
	}

	boxHeight = height - lipgloss.Height(header) - lipgloss.Height(footer)
	return header, footer, boxHeight
}

// renderPhaseLayout renders a full-screen phase view: branding line at top,
// content centered in a bordered box, shortcut footer at bottom.
func renderPhaseLayout(content string, width, height int, footerHint string) string {
	header, footer, boxHeight := phaseHeaderFooter(width, height, footerHint)

	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BorderColor).
		Padding(1, 3).
		Width(width).
		Height(boxHeight).
		AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center)

	contentWidth := lipgloss.Width(content)
	normalized := lipgloss.NewStyle().Width(contentWidth).Render(content)

	return header + "\n" + contentBox.Render(normalized) + "\n" + footer
}

// renderDockerLogs renders a full-screen log view without the mascot card.
func (m Model) renderDockerLogs(title, footerHint string) string {
	header, footer, boxHeight := phaseHeaderFooter(m.width, m.height, footerHint)

	// border (2) + padding (2) + title (1) + blank (1) = 6 lines overhead
	visibleLines := boxHeight - 6
	if visibleLines < 1 {
		visibleLines = 1
	}

	var body strings.Builder
	body.WriteString(panelHeaderStyle.Render(title))
	body.WriteString("\n\n")

	start := 0
	if len(m.overlayLines) > visibleLines {
		start = len(m.overlayLines) - visibleLines
	}
	for _, line := range m.overlayLines[start:] {
		body.WriteString(line + "\n")
	}
	if len(m.overlayLines) == 0 {
		body.WriteString(helpStyle.Render("Waiting for command output..."))
	}

	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BorderColor).
		Padding(1, 3).
		Width(m.width).
		Height(boxHeight)

	return header + "\n" + contentBox.Render(body.String()) + "\n" + footer
}

// overlayMaxLines returns the maximum number of log lines that fit in the overlay.
func (m Model) overlayMaxLines() int {
	if m.height <= 0 {
		return 10
	}
	// Account for border (2), padding (2), title (1), blank line after title (1)
	const overhead = 6
	maxLines := m.height - 2 - overhead
	if maxLines < 10 {
		return 10
	}
	return maxLines
}

func (m Model) renderInstallPrompt(b *strings.Builder) {
	switch m.install.step {
	case installStepAsk:
		warnStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor)
		b.WriteString(warnStyle.Render("Shopware is not installed"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("This project has not been set up yet. The installation\nwill create the database, run migrations and configure\nyour local development environment."))
		b.WriteString("\n\n")
		b.WriteString(renderConfirmButtons("Yes, install now", "No, skip", m.install.confirmYes))

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
		b.WriteString(tui.DimStyle.Render("Enter the password for the admin account"))
		b.WriteString("\n\n")
		b.WriteString(m.install.password.View())
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
		return tui.ShortcutBar(
			tui.Shortcut{Key: "enter", Label: "Install"},
		)
	}
	return ""
}

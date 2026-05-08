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

	switch m.phase {
	case phaseDashboard:
		v.Content = m.renderDashboard()
	case phaseStarting, phaseStopping, phaseInstallPrompt, phaseInstalling:
		v.Content = m.renderPhase()
	case phaseTask:
		v.Content = m.renderDockerLogs(m.taskTitle, "")
	}

	if m.modal != nil {
		v.Content = m.modal.View(m.width, m.height)
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
	case tabConfig:
		content = m.configTab.View(m.width, boxHeight)
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

	if m.activeTab == tabConfig {
		shortcuts := []tui.Shortcut{
			{Key: "↑/↓", Label: "Navigate"},
			{Key: "enter", Label: "Edit/Save"},
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

func (m Model) renderPhase() string {
	var content strings.Builder
	var footerHint string

	switch m.phase {
	case phaseStarting:
		footerHint = tui.ShortcutBadge("l", "Toggle logs")
		if m.dockerShowLogs {
			return m.renderDockerLogs("Starting Docker containers...", footerHint)
		}
		cardContent := fmt.Sprintf("%s Starting Docker containers...", m.dockerSpinner.View())
		content.WriteString(tui.RenderPhaseCard(cardContent))
	case phaseStopping:
		footerHint = tui.ShortcutBadge("l", "Toggle logs")
		if m.dockerShowLogs {
			return m.renderDockerLogs("Stopping Docker containers...", footerHint)
		}
		cardContent := fmt.Sprintf("%s Stopping Docker containers...", m.dockerSpinner.View())
		content.WriteString(tui.RenderPhaseCard(cardContent))
	case phaseInstallPrompt:
		var card strings.Builder
		m.renderInstallPrompt(&card)
		content.WriteString(tui.RenderPhaseCard(card.String()))
		footerHint = m.installFooterHint()
	case phaseInstalling:
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
	case phaseDashboard, phaseTask:
	}

	return renderPhaseLayout(content.String(), m.width, m.height, footerHint)
}

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

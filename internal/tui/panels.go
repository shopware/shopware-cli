package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// RenderPanel renders content inside a full-width bordered box with an optional title.
func RenderPanel(title, content string, titleColor color.Color) string {
	w := TerminalWidth()
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderColor).
		Padding(1, 3).
		Width(w)

	box := boxStyle.Render(strings.TrimRight(content, "\n"))

	if title == "" {
		return box
	}

	lines := strings.Split(box, "\n")

	bStyle := lipgloss.NewStyle().Foreground(BorderColor)
	tStyle := lipgloss.NewStyle().Bold(true).Foreground(titleColor)

	styledTitle := tStyle.Render(title)
	fill := w - 5 - lipgloss.Width(styledTitle)
	if fill < 0 {
		fill = 0
	}

	lines[0] = bStyle.Render("╭─ ") + styledTitle + bStyle.Render(" "+strings.Repeat("─", fill)+"╮")

	return strings.Join(lines, "\n")
}

// RenderSuccessPanel renders a panel with a green "Project Created" title.
func RenderSuccessPanel(content string) string {
	return RenderPanel("✓ Project Created", content, SuccessColor)
}

// RenderCancelledMessage renders a dimmed cancellation notice.
func RenderCancelledMessage() string {
	return "\n" + DimStyle.Render("  Project creation cancelled.") + "\n"
}

// DividerLine renders a horizontal rule spanning most of the terminal width.
func DividerLine() string {
	w := TerminalWidth() - 8
	return lipgloss.NewStyle().
		Foreground(BorderColor).
		Render(strings.Repeat("─", w))
}

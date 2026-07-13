package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// StatusBadge renders a status indicator in the form "● STATUS".
func StatusBadge(status string, c color.Color) string {
	return lipgloss.NewStyle().Foreground(c).Bold(true).Render("● " + strings.ToUpper(status))
}

// TextBadge renders text on a subtle background with horizontal padding.
func TextBadge(text string) string {
	return lipgloss.NewStyle().
		Background(SubtleBgColor).
		Foreground(TextColor).
		Padding(0, 1).
		Render(text)
}

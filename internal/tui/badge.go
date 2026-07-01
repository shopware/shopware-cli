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

// CheckboxRow renders a KVRow whose key is a colored "[glyph] label" checkbox
// and whose value is the status text in the same color. It is the shared layout
// for the Watchers and Background-processing status lists. When bold is true the
// glyph and status are emphasised (used for the "running" state).
func CheckboxRow(glyph, label, status string, c color.Color, bold bool) string {
	glyphStyle := lipgloss.NewStyle().Foreground(c)
	statusStyle := lipgloss.NewStyle().Foreground(c)
	if bold {
		glyphStyle = glyphStyle.Bold(true)
		statusStyle = statusStyle.Bold(true)
	}

	return KVRow(glyphStyle.Render(glyph)+" "+label, statusStyle.Render(status))
}

// TextBadge renders text on a subtle background with horizontal padding.
func TextBadge(text string) string {
	return lipgloss.NewStyle().
		Background(SubtleBgColor).
		Foreground(TextColor).
		Padding(0, 1).
		Render(text)
}

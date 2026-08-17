package tui

import (
	"charm.land/lipgloss/v2"
)

// Checkbox renders a "[x]"/"[ ]" toggle with its label — muted by default,
// brand-highlighted when focused.
func Checkbox(checked, focused bool, label string) string {
	box := "[ ]"
	if checked {
		box = "[x]"
	}
	style := lipgloss.NewStyle().Foreground(MutedColor)
	if focused {
		style = lipgloss.NewStyle().Foreground(BrandColor).Bold(true)
	}
	return style.Render(box + " " + label)
}

package tui

import (
	"charm.land/lipgloss/v2"
)

// fixedIndicator renders an indicator character inside a fixed 2-character-wide column.
func fixedIndicator(indicator string) string {
	return lipgloss.NewStyle().Width(2).Render(indicator)
}

package tui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// TruncateToWidth truncates a string to fit within maxW rune positions,
// appending "…" if truncation occurs.
func TruncateToWidth(s string, maxW int) string {
	if maxW <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxW {
		return s
	}
	if maxW <= 1 {
		return "…"
	}
	return string(runes[:maxW-1]) + "…"
}

// TableCell renders a fixed-width cell with horizontal padding of 1 on each side.
func TableCell(content string, width int, fg color.Color, selected bool) string {
	s := lipgloss.NewStyle().Padding(0, 1).Width(width)
	if selected {
		s = s.Background(SelectedBgColor)
	}
	if fg != nil {
		s = s.Foreground(fg)
	}
	return s.Render(TruncateToWidth(content, width-2))
}

// TableDivider renders a full-width horizontal line in the border color.
func TableDivider(width int) string {
	return lipgloss.NewStyle().Foreground(BorderColor).Render(strings.Repeat("─", width))
}

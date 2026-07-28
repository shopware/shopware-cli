package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// BrandingHeader renders the branding line right-aligned within width — the
// header row shared by the project dev phase screens and the upgrade wizard.
func BrandingHeader(width int) string {
	branding := BrandingLine()
	fill := width - BrandingLineWidth()
	if fill < 0 {
		fill = 0
	}
	return strings.Repeat(" ", fill) + branding
}

// PhaseFooter renders the footer row shared by the project dev phase screens
// and the upgrade wizard: an optional shortcut hint followed by an exit badge
// (usually "Exit"; "Cancel" while a job is running).
func PhaseFooter(hint, exitLabel string) string {
	exit := ShortcutBadge("ctrl+c", exitLabel)
	if hint == "" {
		return exit
	}
	sep := lipgloss.NewStyle().Foreground(BorderColor).Render("  │  ")
	return hint + sep + exit
}

package tui

import (
	"charm.land/lipgloss/v2"
)

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

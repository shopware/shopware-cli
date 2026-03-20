package tui

import (
	"charm.land/lipgloss/v2"
)

// Shortcut describes a single keyboard shortcut to display in a footer bar.
type Shortcut struct {
	Key   string
	Label string
}

var (
	badgeKeyStyle = lipgloss.NewStyle().
			Foreground(TextColor).
			Background(SubtleBgColor).
			Padding(0, 1)

	badgeDescStyle = lipgloss.NewStyle().
			Foreground(MutedColor)
)

// ShortcutBadge renders a single keyboard shortcut as a styled badge.
func ShortcutBadge(key, label string) string {
	return badgeKeyStyle.Render(key) + badgeDescStyle.Render(" "+label)
}

// ShortcutBar joins multiple shortcuts into a horizontal bar separated by dividers.
func ShortcutBar(shortcuts ...Shortcut) string {
	if len(shortcuts) == 0 {
		return ""
	}

	sep := lipgloss.NewStyle().Foreground(BorderColor).Render("  │  ")
	result := ShortcutBadge(shortcuts[0].Key, shortcuts[0].Label)
	for _, s := range shortcuts[1:] {
		result += sep + ShortcutBadge(s.Key, s.Label)
	}
	return result
}

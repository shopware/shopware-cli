package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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
	return shortcutBar("  │  ", shortcuts)
}

// ShortcutBarFit renders a shortcut bar that stays on a single row within
// maxWidth: it falls back to narrower separators when the default bar is too
// wide and truncates with an ellipsis as a last resort. A maxWidth <= 0 means
// unconstrained.
func ShortcutBarFit(maxWidth int, shortcuts ...Shortcut) string {
	bar := shortcutBar("  │  ", shortcuts)
	if maxWidth <= 0 || lipgloss.Width(bar) <= maxWidth {
		return bar
	}

	bar = shortcutBar(" │ ", shortcuts)
	if lipgloss.Width(bar) <= maxWidth {
		return bar
	}

	return ansi.Truncate(bar, maxWidth, "…")
}

func shortcutBar(separator string, shortcuts []Shortcut) string {
	if len(shortcuts) == 0 {
		return ""
	}

	sep := lipgloss.NewStyle().Foreground(BorderColor).Render(separator)
	result := ShortcutBadge(shortcuts[0].Key, shortcuts[0].Label)
	for _, s := range shortcuts[1:] {
		result += sep + ShortcutBadge(s.Key, s.Label)
	}
	return result
}

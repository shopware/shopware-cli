package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func instanceTabShortcuts() []Shortcut {
	return []Shortcut{
		{Key: "↑/↓", Label: "Navigate"},
		{Key: "enter", Label: "Select source"},
		{Key: "pgup/pgdn", Label: "Scroll logs"},
		{Key: "f", Label: "Follow logs"},
		{Key: "tab", Label: "Next tab"},
		{Key: "ctrl+c", Label: "Exit"},
	}
}

func TestShortcutBarFit_UnconstrainedMatchesShortcutBar(t *testing.T) {
	shortcuts := instanceTabShortcuts()

	assert.Equal(t, ShortcutBar(shortcuts...), ShortcutBarFit(0, shortcuts...))
	assert.Equal(t, ShortcutBar(shortcuts...), ShortcutBarFit(500, shortcuts...))
}

func TestShortcutBarFit_NarrowSeparatorsOn120Columns(t *testing.T) {
	shortcuts := instanceTabShortcuts()
	assert.Greater(t, lipgloss.Width(ShortcutBar(shortcuts...)), 120)

	bar := ShortcutBarFit(120, shortcuts...)

	assert.LessOrEqual(t, lipgloss.Width(bar), 120)
	assert.Equal(t, 1, lipgloss.Height(bar))
	// All shortcuts survive; only the separators shrink.
	assert.Contains(t, bar, "Follow logs")
	assert.Contains(t, bar, "Exit")
}

func TestShortcutBarFit_TruncatesWhenNothingFits(t *testing.T) {
	shortcuts := instanceTabShortcuts()

	bar := ShortcutBarFit(40, shortcuts...)

	assert.LessOrEqual(t, lipgloss.Width(bar), 40)
	assert.Equal(t, 1, lipgloss.Height(bar))
	assert.True(t, strings.Contains(bar, "…"))
}

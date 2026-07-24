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

// ShortcutsOptions configure a Shortcuts bar.
type ShortcutsOptions struct {
	Items []Shortcut
	// MaxWidth keeps the bar on a single row within this width: narrower
	// separators first, ellipsis truncation as a last resort. <= 0 means
	// unconstrained.
	MaxWidth int
}

// Shortcuts is a horizontal bar of keyboard shortcut badges.
type Shortcuts struct {
	opts ShortcutsOptions
}

// NewShortcuts creates a shortcut bar.
func NewShortcuts(opts ShortcutsOptions) Shortcuts {
	return Shortcuts{opts: opts}
}

// Render implements the component contract.
func (s Shortcuts) Render() string {
	bar := s.bar("  │  ")
	if s.opts.MaxWidth <= 0 || lipgloss.Width(bar) <= s.opts.MaxWidth {
		return bar
	}

	bar = s.bar(" │ ")
	if lipgloss.Width(bar) <= s.opts.MaxWidth {
		return bar
	}

	return ansi.Truncate(bar, s.opts.MaxWidth, "…")
}

func (s Shortcuts) bar(separator string) string {
	if len(s.opts.Items) == 0 {
		return ""
	}

	sep := lipgloss.NewStyle().Foreground(BorderColor).Render(separator)
	result := s.badge(s.opts.Items[0])
	for _, item := range s.opts.Items[1:] {
		result += sep + s.badge(item)
	}
	return result
}

func (s Shortcuts) badge(item Shortcut) string {
	key := lipgloss.NewStyle().
		Foreground(TextColor).
		Background(SubtleBgColor).
		Padding(0, 1).
		Render(item.Key)
	return key + DimStyle.Render(" "+item.Label)
}

// ShortcutBadge renders a single keyboard shortcut as a styled badge.
func ShortcutBadge(key, label string) string {
	return Shortcuts{}.badge(Shortcut{Key: key, Label: label})
}

// ShortcutBar joins multiple shortcuts into a horizontal bar separated by dividers.
func ShortcutBar(shortcuts ...Shortcut) string {
	return NewShortcuts(ShortcutsOptions{Items: shortcuts}).Render()
}

// ShortcutBarFit renders a shortcut bar that stays on a single row within
// maxWidth: it falls back to narrower separators when the default bar is too
// wide and truncates with an ellipsis as a last resort.
func ShortcutBarFit(maxWidth int, shortcuts ...Shortcut) string {
	return NewShortcuts(ShortcutsOptions{Items: shortcuts, MaxWidth: maxWidth}).Render()
}

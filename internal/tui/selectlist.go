package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// SelectOption describes a single option in a select list.
type SelectOption struct {
	Label  string
	Detail string
}

// RenderSelectList renders a titled option list with a ● selector on the
// active item. The whole list is rendered; use RenderSelectListWindowed when
// the option count can grow large.
func RenderSelectList(title, description string, options []SelectOption, cursor int) string {
	return RenderSelectListWindowed(title, description, options, cursor, 0)
}

// RenderSelectListWindowed renders a select list that shows at most maxVisible
// options at once, scrolling to keep the cursor in view. A maxVisible of 0 (or
// any value >= len(options)) renders every option. When the list is windowed a
// "Showing X–Y of N" line is appended so the box height stays fixed regardless
// of how many options exist.
func RenderSelectListWindowed(title, description string, options []SelectOption, cursor, maxVisible int) string {
	var s strings.Builder

	selectorStyle := lipgloss.NewStyle().Foreground(BrandColor)
	selectedStyle := lipgloss.NewStyle().Foreground(BrandColor)

	s.WriteString(TitleStyle.Render(title))
	s.WriteString("\n")
	if description != "" {
		s.WriteString(lipgloss.NewStyle().Foreground(MutedColor).Render(description))
		s.WriteString("\n")
	}
	s.WriteString("\n")

	windowed := maxVisible > 0 && len(options) > maxVisible

	start, end := 0, len(options)
	if windowed {
		// Keep the cursor inside the [start, end) window.
		start = cursor - maxVisible/2
		if start < 0 {
			start = 0
		}
		if start > len(options)-maxVisible {
			start = len(options) - maxVisible
		}
		end = start + maxVisible
	}

	for i := start; i < end; i++ {
		opt := options[i]
		detail := ""
		if opt.Detail != "" {
			detail = " " + DimStyle.Render("("+opt.Detail+")")
		}
		if i == cursor {
			s.WriteString(selectorStyle.Render("● ") + selectedStyle.Render(opt.Label) + detail)
		} else {
			s.WriteString("  " + FormatLabel(opt.Label, opt.Detail))
		}
		s.WriteString("\n")
	}

	if windowed {
		s.WriteString("\n")
		s.WriteString(DimStyle.Render("Showing " + strconv.Itoa(start+1) + "–" + strconv.Itoa(end) + " of " + strconv.Itoa(len(options))))
	}

	return strings.TrimRight(s.String(), "\n")
}

// SelectList is a stateful, keyboard-navigable single-select list. It owns the
// cursor and windowing so callers only forward key strings and render. Use it
// for any fixed-height option picker (e.g. the project upgrade version select).
type SelectList struct {
	title       string
	description string
	options     []SelectOption
	cursor      int
	maxVisible  int
}

// NewSelectList builds a SelectList. maxVisible caps how many options are shown
// at once (0 = show all); the window scrolls to keep the cursor visible.
func NewSelectList(title, description string, options []SelectOption, maxVisible int) *SelectList {
	return &SelectList{
		title:       title,
		description: description,
		options:     options,
		maxVisible:  maxVisible,
	}
}

// Cursor returns the index of the currently highlighted option.
func (l *SelectList) Cursor() int { return l.cursor }

// Selected returns the currently highlighted option and whether one exists
// (false when the list is empty).
func (l *SelectList) Selected() (SelectOption, bool) {
	if l.cursor < 0 || l.cursor >= len(l.options) {
		return SelectOption{}, false
	}
	return l.options[l.cursor], true
}

// HandleKey applies a navigation key (up/down/k/j, pgup/pgdown, home/g, end/G)
// and reports whether it moved or consumed the cursor. Keys the list does not
// own (enter, esc, …) return false so the caller can act on them.
func (l *SelectList) HandleKey(key string) bool {
	last := len(l.options) - 1
	if last < 0 {
		return false
	}

	switch key {
	case "up", "k":
		l.cursor--
	case "down", "j":
		l.cursor++
	case "pgup":
		l.cursor -= l.page()
	case "pgdown":
		l.cursor += l.page()
	case "home", "g":
		l.cursor = 0
	case "end", "G":
		l.cursor = last
	default:
		return false
	}

	if l.cursor < 0 {
		l.cursor = 0
	}
	if l.cursor > last {
		l.cursor = last
	}
	return true
}

// page is the jump distance for pgup/pgdown.
func (l *SelectList) page() int {
	if l.maxVisible > 0 {
		return l.maxVisible
	}
	return len(l.options)
}

// windowed reports whether the option count exceeds the visible window.
func (l *SelectList) windowed() bool {
	return l.maxVisible > 0 && len(l.options) > l.maxVisible
}

// View renders the list at its current cursor position.
func (l *SelectList) View() string {
	return RenderSelectListWindowed(l.title, l.description, l.options, l.cursor, l.maxVisible)
}

// Shortcuts returns the navigation hints to show in a footer. PgUp/PgDn is only
// included when the list is windowed.
func (l *SelectList) Shortcuts() []Shortcut {
	shortcuts := []Shortcut{{Key: "↑/↓", Label: "Select"}}
	if l.windowed() {
		shortcuts = append(shortcuts, Shortcut{Key: "PgUp/PgDn", Label: "Jump"})
	}
	return shortcuts
}

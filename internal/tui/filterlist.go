package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// FilterItem is one selectable entry of a FilterList.
type FilterItem struct {
	// Label is the row text; Detail is dimmed, right-aligned extra text.
	// Both are matched by the filter.
	Label  string
	Detail string
	// Value is an optional payload identifier for the caller.
	Value string
}

// FilterListOptions configure a FilterList.
type FilterListOptions struct {
	Items []FilterItem
	// PageSize is the number of visible rows (default 10).
	PageSize int
	// Placeholder for the filter input.
	Placeholder string
	// InitialIndex preselects an item by its index in Items.
	InitialIndex int
	// Header is an optional column-header line rendered above the rows.
	Header string
}

// FilterList is a type-to-filter list with a cursor and a windowed view — the
// shared mechanics behind pickers and palettes. It handles up/down movement
// and filter input; enter/esc handling stays with the caller, which reads the
// choice via Selected.
type FilterList struct {
	opts     FilterListOptions
	filter   textinput.Model
	filtered []int
	cursor   int
	scroll   int
}

// NewFilterList creates a filter list.
func NewFilterList(opts FilterListOptions) FilterList {
	if opts.PageSize <= 0 {
		opts.PageSize = 10
	}

	ti := textinput.New()
	ti.Prompt = lipgloss.NewStyle().Foreground(BrandColor).Render("> ")
	ti.Placeholder = opts.Placeholder
	if ti.Placeholder == "" {
		ti.Placeholder = "Type to filter"
	}
	ti.CharLimit = 64
	ti.Focus()

	l := FilterList{opts: opts, filter: ti}
	l.applyFilter()

	for i, idx := range l.filtered {
		if idx == opts.InitialIndex {
			l.cursor = i
			break
		}
	}
	l.clampScroll()
	return l
}

// Init returns the cursor blink command for the filter input.
func (l FilterList) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles cursor movement and filter typing. Enter and esc are not
// consumed — the caller decides what selection and dismissal mean.
func (l FilterList) Update(msg tea.Msg) (FilterList, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch KeyString(key) {
		case KeyUp:
			if l.cursor > 0 {
				l.cursor--
				l.clampScroll()
			}
			return l, nil
		case KeyDown:
			if l.cursor < len(l.filtered)-1 {
				l.cursor++
				l.clampScroll()
			}
			return l, nil
		}
	}

	var cmd tea.Cmd
	l.filter, cmd = l.filter.Update(msg)
	l.applyFilter()
	return l, cmd
}

// Selected returns the item under the cursor and its index in Items.
func (l FilterList) Selected() (FilterItem, int, bool) {
	if len(l.filtered) == 0 {
		return FilterItem{}, -1, false
	}
	idx := l.filtered[l.cursor]
	return l.opts.Items[idx], idx, true
}

// Len returns the number of items matching the current filter.
func (l FilterList) Len() int {
	return len(l.filtered)
}

// Items returns all items the list was created with.
func (l FilterList) Items() []FilterItem {
	return l.opts.Items
}

// View renders the filter input, the visible window of rows, and a
// "Showing x of y" line when the list overflows the window.
func (l FilterList) View(width int) string {
	var b strings.Builder
	b.WriteString(l.filter.View())
	b.WriteString("\n\n")

	if l.opts.Header != "" {
		b.WriteString(BoldStyle.Render(l.opts.Header))
		b.WriteString("\n")
	}

	selectedStyle := lipgloss.NewStyle().
		Foreground(BrandColor).
		Background(SelectedBgColor).
		Bold(true).
		Width(width)
	normalStyle := lipgloss.NewStyle().Foreground(TextColor).Width(width)
	detailStyle := lipgloss.NewStyle().Foreground(MutedColor)
	selectedDetailStyle := lipgloss.NewStyle().Foreground(MutedColor).Background(SelectedBgColor)

	end := min(l.scroll+l.opts.PageSize, len(l.filtered))
	for i := l.scroll; i < end; i++ {
		item := l.opts.Items[l.filtered[i]]
		rowStyle, dStyle := normalStyle, detailStyle
		if i == l.cursor {
			rowStyle, dStyle = selectedStyle, selectedDetailStyle
		}
		if item.Detail != "" {
			gap := max(width-lipgloss.Width(item.Label)-lipgloss.Width(item.Detail), 1)
			b.WriteString(rowStyle.Render(item.Label + strings.Repeat(" ", gap) + dStyle.Render(item.Detail)))
		} else {
			b.WriteString(rowStyle.Render(item.Label))
		}
		b.WriteString("\n")
	}

	switch {
	case len(l.filtered) == 0:
		b.WriteString(DimStyle.Render("No matching items"))
		b.WriteString("\n")
	case len(l.filtered) > l.opts.PageSize:
		b.WriteString(DimStyle.Render(fmt.Sprintf("Showing %d–%d of %d", l.scroll+1, end, len(l.filtered))))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func (l *FilterList) applyFilter() {
	query := strings.ToLower(l.filter.Value())
	l.filtered = l.filtered[:0]
	for i, item := range l.opts.Items {
		if query == "" ||
			strings.Contains(strings.ToLower(item.Label), query) ||
			strings.Contains(strings.ToLower(item.Detail), query) {
			l.filtered = append(l.filtered, i)
		}
	}
	if l.cursor >= len(l.filtered) {
		l.cursor = max(len(l.filtered)-1, 0)
	}
	l.clampScroll()
}

func (l *FilterList) clampScroll() {
	if l.cursor < l.scroll {
		l.scroll = l.cursor
	}
	if l.cursor >= l.scroll+l.opts.PageSize {
		l.scroll = l.cursor - l.opts.PageSize + 1
	}
	if l.scroll < 0 {
		l.scroll = 0
	}
}

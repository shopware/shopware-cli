package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// FilterMultiSelectItem is a single row presented by FilterMultiSelect. Label
// is the primary text, Detail is an optional secondary line (e.g. a path).
// Both are matched against the filter query. Value is the string returned when
// the user selects this item.
type FilterMultiSelectItem struct {
	Label  string
	Detail string
	Value  string
}

// FilterMultiSelect renders an interactive picker with a type-to-filter input
// and a windowed list that allows toggling multiple items. It returns the
// chosen Values in the original item order, or ErrFilterSelectCancelled if the
// user dismisses the prompt.
//
// Use it from CLI commands that need to pick several entries from a list that
// may be small or huge. For single selection use FilterSelect instead.
func FilterMultiSelect(ctx context.Context, title, help string, items []FilterMultiSelectItem) ([]string, error) {
	if len(items) == 0 {
		return nil, errors.New("no items to choose from")
	}

	ti := textinput.New()
	ti.Prompt = lipgloss.NewStyle().Foreground(BrandColor).Render("> ")
	ti.Placeholder = "Type to filter"
	ti.CharLimit = 64
	ti.Focus()

	m := &filterMultiSelectModel{
		title:    title,
		help:     help,
		items:    items,
		filter:   ti,
		pageSize: filterSelectPageSize,
		selected: make(map[int]bool, len(items)),
	}
	m.applyFilter()

	p := tea.NewProgram(m, tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil {
		return nil, err
	}
	res := final.(*filterMultiSelectModel)
	if res.cancelled {
		return nil, ErrFilterSelectCancelled
	}

	chosen := make([]string, 0, len(res.selected))
	for i, it := range res.items {
		if res.selected[i] {
			chosen = append(chosen, it.Value)
		}
	}
	return chosen, nil
}

type filterMultiSelectModel struct {
	title    string
	help     string
	items    []FilterMultiSelectItem
	filter   textinput.Model
	filtered []int
	selected map[int]bool
	cursor   int
	scroll   int
	pageSize int

	cancelled bool
	width     int
}

func (m *filterMultiSelectModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *filterMultiSelectModel) applyFilter() {
	query := strings.ToLower(m.filter.Value())
	m.filtered = m.filtered[:0]
	for i, it := range m.items {
		if query == "" ||
			strings.Contains(strings.ToLower(it.Label), query) ||
			strings.Contains(strings.ToLower(it.Detail), query) {
			m.filtered = append(m.filtered, i)
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(len(m.filtered)-1, 0)
	}
	m.clampScroll()
}

func (m *filterMultiSelectModel) clampScroll() {
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+m.pageSize {
		m.scroll = m.cursor - m.pageSize + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m *filterMultiSelectModel) toggle() {
	if len(m.filtered) == 0 {
		return
	}
	idx := m.filtered[m.cursor]
	if m.selected[idx] {
		delete(m.selected, idx)
	} else {
		m.selected[idx] = true
	}
}

func (m *filterMultiSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			return m, tea.Quit
		case "space", " ":
			m.toggle()
			return m, nil
		case "up":
			if m.cursor > 0 {
				m.cursor--
				m.clampScroll()
			}
			return m, nil
		case "down":
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				m.clampScroll()
			}
			return m, nil
		}

		var cmd tea.Cmd
		m.filter, cmd = m.filter.Update(msg)
		m.applyFilter()
		return m, cmd
	}
	return m, nil
}

func (m *filterMultiSelectModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m *filterMultiSelectModel) render() string {
	innerWidth := m.width - 6
	if innerWidth < 30 {
		innerWidth = 60
	}
	if innerWidth > 100 {
		innerWidth = 100
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(BrandColor)

	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n")
	if m.help != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(MutedColor).Render(m.help))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.filter.View())
	b.WriteString("\n\n")

	selectedStyle := lipgloss.NewStyle().
		Foreground(BrandColor).
		Background(SelectedBgColor).
		Bold(true).
		Width(innerWidth)
	normalStyle := lipgloss.NewStyle().
		Foreground(TextColor).
		Width(innerWidth)
	detailStyle := lipgloss.NewStyle().Foreground(MutedColor)
	selectedDetailStyle := lipgloss.NewStyle().
		Foreground(MutedColor).
		Background(SelectedBgColor)

	end := m.scroll + m.pageSize
	if end > len(m.filtered) {
		end = len(m.filtered)
	}
	for i := m.scroll; i < end; i++ {
		idx := m.filtered[i]
		item := m.items[idx]
		rowStyle, dStyle := normalStyle, detailStyle
		if i == m.cursor {
			rowStyle, dStyle = selectedStyle, selectedDetailStyle
		}
		check := "[ ] "
		if m.selected[idx] {
			check = "[x] "
		}
		label := check + item.Label
		if item.Detail != "" {
			gap := max(innerWidth-lipgloss.Width(label)-lipgloss.Width(item.Detail), 1)
			b.WriteString(rowStyle.Render(label + strings.Repeat(" ", gap) + dStyle.Render(item.Detail)))
		} else {
			b.WriteString(rowStyle.Render(label))
		}
		b.WriteString("\n")
	}
	if len(m.filtered) == 0 {
		b.WriteString(detailStyle.Render("No matching items"))
		b.WriteString("\n")
	} else if len(m.filtered) > m.pageSize {
		b.WriteString(detailStyle.Render("Showing " + strconv.Itoa(m.scroll+1) + "–" + strconv.Itoa(end) + " of " + strconv.Itoa(len(m.filtered))))
		b.WriteString("\n")
	}

	b.WriteString(detailStyle.Render(strconv.Itoa(len(m.selected)) + " selected"))
	b.WriteString("\n")

	b.WriteString("\n")
	b.WriteString(ShortcutBar(
		Shortcut{Key: "↑/↓", Label: "Move"},
		Shortcut{Key: "space", Label: "Toggle"},
		Shortcut{Key: "enter", Label: "Confirm"},
		Shortcut{Key: "esc", Label: "Cancel"},
	))

	return b.String()
}

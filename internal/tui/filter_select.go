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

// ErrFilterSelectCancelled is returned by FilterSelect when the user dismisses
// the prompt with Esc or Ctrl+C.
var ErrFilterSelectCancelled = errors.New("selection cancelled")

// FilterSelectItem is a single row presented by FilterSelect. Label is the
// primary text, Detail is an optional secondary line (e.g. a URL). Both are
// matched against the filter query. Value is the string returned when the
// user confirms this item.
type FilterSelectItem struct {
	Label  string
	Detail string
	Value  string
}

// FilterSelect renders an interactive picker with a type-to-filter input and
// a windowed list. It returns the chosen Value, or ErrFilterSelectCancelled
// if the user dismisses the prompt.
//
// Use it from CLI commands that need to pick from a list that may be small
// or huge. For the devtui dashboard, use the listPicker modal instead — this
// helper owns its own bubbletea program and is not embeddable.
func FilterSelect(ctx context.Context, title, help string, items []FilterSelectItem) (string, error) {
	if len(items) == 0 {
		return "", errors.New("no items to choose from")
	}

	ti := textinput.New()
	ti.Prompt = lipgloss.NewStyle().Foreground(BrandColor).Render("> ")
	ti.Placeholder = "Type to filter"
	ti.CharLimit = 64
	ti.Focus()

	m := &filterSelectModel{
		title:    title,
		help:     help,
		items:    items,
		filter:   ti,
		pageSize: filterSelectPageSize,
	}
	m.applyFilter()

	p := tea.NewProgram(m, tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	res := final.(*filterSelectModel)
	if res.cancelled {
		return "", ErrFilterSelectCancelled
	}
	if res.chosen < 0 || res.chosen >= len(res.items) {
		return "", ErrFilterSelectCancelled
	}
	return res.items[res.chosen].Value, nil
}

const filterSelectPageSize = 10

type filterSelectModel struct {
	title    string
	help     string
	items    []FilterSelectItem
	filter   textinput.Model
	filtered []int
	cursor   int
	scroll   int
	pageSize int

	chosen    int
	cancelled bool
	width     int
}

func (m *filterSelectModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *filterSelectModel) applyFilter() {
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

func (m *filterSelectModel) clampScroll() {
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

func (m *filterSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			if len(m.filtered) == 0 {
				m.cancelled = true
				return m, tea.Quit
			}
			m.chosen = m.filtered[m.cursor]
			return m, tea.Quit
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

func (m *filterSelectModel) View() tea.View {
	return tea.NewView(m.render())
}

func (m *filterSelectModel) render() string {
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
		item := m.items[m.filtered[i]]
		rowStyle, dStyle := normalStyle, detailStyle
		if i == m.cursor {
			rowStyle, dStyle = selectedStyle, selectedDetailStyle
		}
		if item.Detail != "" {
			gap := max(innerWidth-lipgloss.Width(item.Label)-lipgloss.Width(item.Detail), 1)
			b.WriteString(rowStyle.Render(item.Label + strings.Repeat(" ", gap) + dStyle.Render(item.Detail)))
		} else {
			b.WriteString(rowStyle.Render(item.Label))
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

	b.WriteString("\n")
	b.WriteString(ShortcutBar(
		Shortcut{Key: "↑/↓", Label: "Choose"},
		Shortcut{Key: "enter", Label: "Confirm"},
		Shortcut{Key: "esc", Label: "Cancel"},
	))

	return b.String()
}

package tui

import (
	"context"
	"errors"
	"strings"

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
// or huge. Inside a hosted TUI, embed FilterList instead — this helper owns
// its own bubbletea program.
func FilterSelect(ctx context.Context, title, help string, items []FilterSelectItem) (string, error) {
	if len(items) == 0 {
		return "", errors.New("no items to choose from")
	}

	filterItems := make([]FilterItem, len(items))
	for i, item := range items {
		filterItems[i] = FilterItem(item)
	}

	m := &filterSelectModel{
		title: title,
		help:  help,
		list:  NewFilterList(FilterListOptions{Items: filterItems}),
	}

	p := tea.NewProgram(m, tea.WithContext(ctx))
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	res := final.(*filterSelectModel)
	if res.cancelled || res.chosen < 0 {
		return "", ErrFilterSelectCancelled
	}
	return items[res.chosen].Value, nil
}

type filterSelectModel struct {
	title string
	help  string
	list  FilterList

	chosen    int
	cancelled bool
	width     int
}

func (m *filterSelectModel) Init() tea.Cmd {
	m.chosen = -1
	return m.list.Init()
}

func (m *filterSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case tea.KeyPressMsg:
		switch KeyString(msg) {
		case KeyEsc, KeyCtrlC:
			m.cancelled = true
			return m, tea.Quit
		case KeyEnter:
			_, index, ok := m.list.Selected()
			if !ok {
				m.cancelled = true
				return m, tea.Quit
			}
			m.chosen = index
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
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

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(BrandColor).Render(m.title))
	b.WriteString("\n")
	if m.help != "" {
		b.WriteString(DimStyle.Render(m.help))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.list.View(innerWidth))
	b.WriteString("\n\n")
	b.WriteString(ShortcutBar(
		Shortcut{Key: "↑/↓", Label: "Choose"},
		Shortcut{Key: "enter", Label: "Confirm"},
		Shortcut{Key: "esc", Label: "Cancel"},
	))

	return b.String()
}

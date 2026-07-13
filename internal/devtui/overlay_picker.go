package devtui

import (
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

type pickerResultMsg struct {
	Key       any
	Cancelled bool
	Value     string
	Index     int
}

type listPickerItem struct {
	Label  string
	Detail string
	Value  string
}

type listPicker struct {
	key      any
	title    string
	help     string
	items    []listPickerItem
	filter   textinput.Model
	filtered []int
	cursor   int
	scroll   int
	pageSize int
}

const listPickerPageSize = 10

func newListPicker(key any, title, help string, items []listPickerItem, initialIndex int) *listPicker {
	ti := textinput.New()
	ti.Prompt = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("> ")
	ti.Placeholder = "Type to filter"
	ti.CharLimit = 64
	ti.Focus()

	lp := &listPicker{
		key:      key,
		title:    title,
		help:     help,
		items:    items,
		filter:   ti,
		pageSize: listPickerPageSize,
	}
	lp.applyFilter()

	for i, idx := range lp.filtered {
		if idx == initialIndex {
			lp.cursor = i
			break
		}
	}
	lp.clampScroll()
	return lp
}

func (lp *listPicker) applyFilter() {
	query := strings.ToLower(lp.filter.Value())
	lp.filtered = lp.filtered[:0]
	for i, it := range lp.items {
		if query == "" ||
			strings.Contains(strings.ToLower(it.Label), query) ||
			strings.Contains(strings.ToLower(it.Detail), query) {
			lp.filtered = append(lp.filtered, i)
		}
	}
	if lp.cursor >= len(lp.filtered) {
		lp.cursor = max(len(lp.filtered)-1, 0)
	}
	lp.clampScroll()
}

func (lp *listPicker) clampScroll() {
	if lp.cursor < lp.scroll {
		lp.scroll = lp.cursor
	}
	if lp.cursor >= lp.scroll+lp.pageSize {
		lp.scroll = lp.cursor - lp.pageSize + 1
	}
	if lp.scroll < 0 {
		lp.scroll = 0
	}
}

func (lp *listPicker) selected() (listPickerItem, int, bool) {
	if len(lp.filtered) == 0 {
		return listPickerItem{}, -1, false
	}
	idx := lp.filtered[lp.cursor]
	return lp.items[idx], idx, true
}

func (lp *listPicker) Update(msg tea.Msg) (Modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return lp, nil
	}

	switch key.String() {
	case "esc":
		return nil, emit(pickerResultMsg{Key: lp.key, Cancelled: true})
	case keyEnter:
		item, idx, ok := lp.selected()
		if !ok {
			return nil, emit(pickerResultMsg{Key: lp.key, Cancelled: true})
		}
		return nil, emit(pickerResultMsg{Key: lp.key, Value: item.Value, Index: idx})
	case keyUp:
		if lp.cursor > 0 {
			lp.cursor--
			lp.clampScroll()
		}
		return lp, nil
	case keyDown:
		if lp.cursor < len(lp.filtered)-1 {
			lp.cursor++
			lp.clampScroll()
		}
		return lp, nil
	}

	var cmd tea.Cmd
	lp.filter, cmd = lp.filter.Update(msg)
	lp.applyFilter()
	return lp, cmd
}

func (lp *listPicker) View(width, height int) string {
	modalWidth := min(width-4, 70)
	innerWidth := modalWidth - 6

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor)

	var b strings.Builder
	b.WriteString(titleStyle.Render(lp.title))
	b.WriteString("\n\n")
	if lp.help != "" {
		b.WriteString(helpStyle.Render(lp.help))
		b.WriteString("\n\n")
	}

	b.WriteString(lp.filter.View())
	b.WriteString("\n\n")

	selectedStyle := lipgloss.NewStyle().
		Foreground(tui.BrandColor).
		Background(tui.SelectedBgColor).
		Bold(true).
		Width(innerWidth)
	normalStyle := lipgloss.NewStyle().
		Foreground(tui.TextColor).
		Width(innerWidth)
	detailStyle := lipgloss.NewStyle().Foreground(tui.MutedColor)
	selectedDetailStyle := lipgloss.NewStyle().
		Foreground(tui.MutedColor).
		Background(tui.SelectedBgColor)

	end := lp.scroll + lp.pageSize
	if end > len(lp.filtered) {
		end = len(lp.filtered)
	}

	for i := lp.scroll; i < end; i++ {
		idx := lp.filtered[i]
		item := lp.items[idx]
		rowStyle, dStyle := normalStyle, detailStyle
		if i == lp.cursor {
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
	if len(lp.filtered) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(tui.MutedColor).Render("No matching items"))
		b.WriteString("\n")
	} else if len(lp.filtered) > lp.pageSize {
		b.WriteString(detailStyle.Render("Showing " + strconv.Itoa(lp.scroll+1) + "–" + strconv.Itoa(end) + " of " + strconv.Itoa(len(lp.filtered))))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(tui.ShortcutBar(
		tui.Shortcut{Key: "↑/↓", Label: "Choose"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "esc", Label: "Cancel"},
	))

	return centeredModal(b.String(), modalWidth, width, height)
}

type textPicker struct {
	key    any
	title  string
	help   string
	input  textinput.Model
	secret bool
}

func newTextPicker(key any, title, help, value string, secret bool) *textPicker {
	ti := textinput.New()
	ti.Placeholder = title
	ti.CharLimit = 128
	ti.Prompt = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("> ")
	if value != "" {
		ti.SetValue(value)
	}
	ti.Focus()
	return &textPicker{key: key, title: title, help: help, input: ti, secret: secret}
}

func (tp *textPicker) Update(msg tea.Msg) (Modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return tp, nil
	}
	switch key.String() {
	case "esc":
		return nil, emit(pickerResultMsg{Key: tp.key, Cancelled: true})
	case keyEnter:
		return nil, emit(pickerResultMsg{Key: tp.key, Value: tp.input.Value()})
	}
	var cmd tea.Cmd
	tp.input, cmd = tp.input.Update(msg)
	return tp, cmd
}

func (tp *textPicker) View(width, height int) string {
	modalWidth := min(width-4, 70)

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor)

	var b strings.Builder
	b.WriteString(titleStyle.Render(tp.title))
	b.WriteString("\n\n")
	if tp.help != "" {
		b.WriteString(helpStyle.Render(tp.help))
		b.WriteString("\n\n")
	}
	b.WriteString(tp.input.View())
	b.WriteString("\n\n")
	b.WriteString(tui.ShortcutBar(
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "esc", Label: "Cancel"},
	))

	return centeredModal(b.String(), modalWidth, width, height)
}

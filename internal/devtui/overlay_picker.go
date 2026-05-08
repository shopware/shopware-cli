package devtui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

type valuePickerKind int

const (
	valuePickerList valuePickerKind = iota
	valuePickerText
)

// valuePickerResultMsg is emitted when the picker is dismissed. Cancelled is
// true if Esc was pressed; otherwise Field/Value carry the chosen value.
type valuePickerResultMsg struct {
	Cancelled bool
	Field     configField
	Value     string
}

// valuePicker is a modal that edits a single config field. It renders either
// a selectable list of options or a text input, depending on kind.
type valuePicker struct {
	kind   valuePickerKind
	field  configField
	title  string
	help   string
	secret bool

	options []string // list mode
	labels  []string // optional pretty labels; falls back to options
	cursor  int

	input textinput.Model // text mode
}

func newListPicker(field configField, title string, options []string, labels []string, current int) *valuePicker {
	return &valuePicker{
		kind:    valuePickerList,
		field:   field,
		title:   title,
		options: options,
		labels:  labels,
		cursor:  current,
	}
}

func newTextPicker(field configField, title, help, value string, secret bool) *valuePicker {
	ti := textinput.New()
	ti.Placeholder = title
	ti.CharLimit = 128
	ti.Prompt = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("> ")
	if value != "" {
		ti.SetValue(value)
	}
	ti.Focus()
	return &valuePicker{
		kind:   valuePickerText,
		field:  field,
		title:  title,
		help:   help,
		secret: secret,
		input:  ti,
	}
}

func (vp *valuePicker) selectedValue() string {
	if vp.kind == valuePickerText {
		return vp.input.Value()
	}
	if len(vp.options) == 0 {
		return ""
	}
	return vp.options[vp.cursor]
}

func (vp *valuePicker) optionLabel(i int) string {
	if i < len(vp.labels) && vp.labels[i] != "" {
		return vp.labels[i]
	}
	if i < len(vp.options) {
		return vp.options[i]
	}
	return ""
}

func (vp *valuePicker) Update(msg tea.Msg) (Modal, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return vp, nil
	}

	switch key.String() {
	case "esc":
		return nil, emit(valuePickerResultMsg{Cancelled: true, Field: vp.field})
	case keyEnter:
		return nil, emit(valuePickerResultMsg{Field: vp.field, Value: vp.selectedValue()})
	}

	if vp.kind == valuePickerList {
		switch key.String() {
		case keyUp, keyK:
			if vp.cursor > 0 {
				vp.cursor--
			}
		case keyDown, keyJ:
			if vp.cursor < len(vp.options)-1 {
				vp.cursor++
			}
		}
		return vp, nil
	}

	var cmd tea.Cmd
	vp.input, cmd = vp.input.Update(msg)
	return vp, cmd
}

func (vp *valuePicker) View(width, height int) string {
	modalWidth := min(width-4, 70)
	innerWidth := modalWidth - 6

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor)

	var b strings.Builder
	b.WriteString(titleStyle.Render(vp.title))
	b.WriteString("\n\n")
	if vp.help != "" {
		b.WriteString(helpStyle.Render(vp.help))
		b.WriteString("\n\n")
	}

	switch vp.kind {
	case valuePickerList:
		selectedStyle := lipgloss.NewStyle().
			Foreground(tui.BrandColor).
			Background(tui.SelectedBgColor).
			Bold(true).
			Width(innerWidth)
		normalStyle := lipgloss.NewStyle().
			Foreground(tui.TextColor).
			Width(innerWidth)

		for i := range vp.options {
			label := vp.optionLabel(i)
			if i == vp.cursor {
				b.WriteString(selectedStyle.Render(label))
			} else {
				b.WriteString(normalStyle.Render(label))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(tui.ShortcutBar(
			tui.Shortcut{Key: "↑/↓", Label: "Choose"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
			tui.Shortcut{Key: "esc", Label: "Cancel"},
		))

	case valuePickerText:
		b.WriteString(vp.input.View())
		b.WriteString("\n\n")
		b.WriteString(tui.ShortcutBar(
			tui.Shortcut{Key: "enter", Label: "Confirm"},
			tui.Shortcut{Key: "esc", Label: "Cancel"},
		))
	}

	return centeredModal(b.String(), modalWidth, width, height)
}

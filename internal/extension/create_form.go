package extension

import (
	"errors"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

// ErrCreationCancelled is returned when the user dismisses the creation form.
var ErrCreationCancelled = errors.New("extension creation cancelled")

const (
	fieldName = iota
	fieldNamespace
	fieldCount
)

var createFieldLabels = map[int]string{
	fieldName:      "Name",
	fieldNamespace: "Namespace",
}

type scaffoldingOption struct {
	id      string
	label   string
	help    string
	checked bool
}

type createPhase int

const (
	phaseForm createPhase = iota
	phaseProgress
	phaseDone
)

// createForm is the interactive creation screen. Name, namespace, and
// scaffolding options stay on one form until the user submits; the same
// program then shows placeholder progress and a success summary.
type createForm struct {
	inputs    []textinput.Model
	options   []scaffoldingOption
	focus     int
	helpOpen  bool
	cancelled bool

	phase    createPhase
	progress progress.Model
	logs     []string
	step     int
	result   CreateOptions
}

func newCreateForm(opts CreateOptions) *createForm {
	name := textinput.New()
	name.Placeholder = "SwagExample"
	name.CharLimit = 64
	name.SetValue(opts.Name)

	namespace := textinput.New()
	namespace.Placeholder = "Swag"
	namespace.CharLimit = 64
	namespace.SetValue(opts.Namespace)

	form := &createForm{
		inputs:  []textinput.Model{name, namespace},
		options: newScaffoldingOptions(opts),
	}
	form.inputs[fieldName].Focus()

	return form
}

func newScaffoldingOptions(opts CreateOptions) []scaffoldingOption {
	return []scaffoldingOption{
		{
			id:      "all-examples",
			label:   "All examples",
			help:    "Placeholder: longer description of the all-examples option.",
			checked: opts.AllExamples,
		},
		{
			id:      "console-command",
			label:   "Console command",
			help:    "Placeholder: longer description of the console-command option.",
			checked: opts.ConsoleCommand,
		},
		{
			id:      "scheduled-task",
			label:   "Scheduled task",
			help:    "Placeholder: longer description of the scheduled-task option.",
			checked: opts.ScheduledTask,
		},
		{
			id:      "event-subscriber",
			label:   "Event subscriber",
			help:    "Placeholder: longer description of the event-subscriber option.",
			checked: opts.EventSubscriber,
		},
		{
			id:      "controller",
			label:   "Controller",
			help:    "Placeholder: longer description of the controller option.",
			checked: opts.Controller,
		},
		{
			id:      "route",
			label:   "Route",
			help:    "Placeholder: longer description of the route option.",
			checked: opts.Route,
		},
		{
			id:      "javascript-plugin",
			label:   "JavaScript plugin",
			help:    "Placeholder: longer description of the javascript-plugin option.",
			checked: opts.JavascriptPlugin,
		},
		{
			id:      "admin-module",
			label:   "Admin module",
			help:    "Placeholder: longer description of the admin-module option.",
			checked: opts.AdminModule,
		},
		{
			id:      "custom-fieldset",
			label:   "Custom fieldset",
			help:    "Placeholder: longer description of the custom-fieldset option.",
			checked: opts.CustomFieldset,
		},
	}
}

func (m *createForm) Init() tea.Cmd {
	return textinput.Blink
}

func (m *createForm) fieldCount() int {
	return fieldCount + len(m.options) + 1
}

func (m *createForm) onInput() bool {
	return m.focus < fieldCount
}

func (m *createForm) onOption() bool {
	i := m.optionIndex()
	return i >= 0 && i < len(m.options)
}

func (m *createForm) onSubmit() bool {
	return m.focus == fieldCount+len(m.options)
}

func (m *createForm) optionIndex() int {
	return m.focus - fieldCount
}

func (m *createForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m.phase {
	case phaseProgress:
		return m.updateProgress(msg)
	case phaseDone:
		return m.updateDone(msg)
	}
	return m.updateForm(msg)
}

func (m *createForm) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if m.onInput() {
			var cmd tea.Cmd
			m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch tui.KeyString(key) {
	case tui.KeyCtrlC:
		m.cancelled = true
		return m, tea.Quit
	case tui.KeyEsc:
		if m.helpOpen {
			m.helpOpen = false
			return m, nil
		}
		m.cancelled = true
		return m, tea.Quit
	case tui.KeyTab, tui.KeyDown:
		return m, m.focusIndex(m.focus + 1)
	case tui.KeyShiftTab, tui.KeyUp:
		return m, m.focusIndex(m.focus - 1)
	case tui.KeyEnter:
		if m.onSubmit() {
			return m, m.beginProgress()
		}
		if m.onInput() {
			return m, m.focusIndex(m.focus + 1)
		}
		m.toggleFocused()
		return m, nil
	case "space", " ":
		if m.onOption() {
			m.toggleFocused()
			return m, nil
		}
		if m.onSubmit() {
			return m, m.beginProgress()
		}
	case "?", "h":
		if m.onOption() {
			m.helpOpen = !m.helpOpen
			return m, nil
		}
	}

	if m.onInput() {
		var cmd tea.Cmd
		m.inputs[m.focus], cmd = m.inputs[m.focus].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m *createForm) toggleFocused() {
	i := m.optionIndex()
	if i < 0 || i >= len(m.options) {
		return
	}
	m.options[i].checked = !m.options[i].checked
}

func (m *createForm) focusIndex(index int) tea.Cmd {
	count := m.fieldCount()
	index = (index + count) % count

	if m.onInput() {
		m.inputs[m.focus].Blur()
	}
	m.focus = index
	m.helpOpen = false

	if m.onInput() {
		return m.inputs[m.focus].Focus()
	}
	return nil
}

func (m *createForm) View() tea.View {
	switch m.phase {
	case phaseProgress:
		return tea.NewView(m.renderProgress())
	case phaseDone:
		return tea.NewView(m.renderDone())
	}

	var b strings.Builder

	b.WriteString(tui.SectionTitleStyle.Render("Create a new extension"))
	b.WriteString("\n\n")
	b.WriteString(m.renderInputs())
	b.WriteString(tui.SectionTitleStyle.Render("Scaffolding options"))
	b.WriteString("\n\n")
	b.WriteString(m.renderOptions())
	b.WriteString(m.renderSubmit())
	b.WriteString(m.renderShortcuts())
	b.WriteString("\n")

	return tea.NewView(b.String())
}

func (m *createForm) renderInputs() string {
	var b strings.Builder

	for i, input := range m.inputs {
		label := tui.DimStyle
		prompt := "  "
		if i == m.focus {
			label = lipgloss.NewStyle().Foreground(tui.BrandColor).Bold(true)
			prompt = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("> ")
		}

		b.WriteString(label.Render(createFieldLabels[i]))
		b.WriteString("\n")
		b.WriteString(prompt)
		b.WriteString(input.View())
		b.WriteString("\n\n")
	}

	return b.String()
}

func (m *createForm) renderOptions() string {
	var b strings.Builder

	for i, opt := range m.options {
		focused := m.focus == fieldCount+i
		b.WriteString(tui.Checkbox(opt.checked, focused, opt.label))
		if focused {
			b.WriteString("  ")
			b.WriteString(tui.DimStyle.Render("[?]"))
		}
		b.WriteString("\n")
		if focused && m.helpOpen {
			b.WriteString("    ")
			b.WriteString(tui.LabelStyle.Render(opt.help))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	return b.String()
}

func (m *createForm) renderSubmit() string {
	active := -1
	if m.onSubmit() {
		active = 0
	}

	return tui.NewButtonRow(tui.ButtonRowOptions{
		Labels: []string{"Submit"},
		Active: active,
	}).Render() + "\n\n"
}

func (m *createForm) renderShortcuts() string {
	if m.onSubmit() {
		return tui.ShortcutBar(
			tui.Shortcut{Key: "tab", Label: "Next field"},
			tui.Shortcut{Key: "enter", Label: "Submit"},
			tui.Shortcut{Key: "esc", Label: "Cancel"},
		)
	}

	shortcuts := []tui.Shortcut{
		{Key: "tab", Label: "Next field"},
		{Key: "space", Label: "Toggle"},
		{Key: "?", Label: "Help"},
		{Key: "esc", Label: "Cancel"},
	}
	if m.helpOpen {
		shortcuts[2] = tui.Shortcut{Key: "?", Label: "Hide help"}
		shortcuts[3] = tui.Shortcut{Key: "esc", Label: "Hide help"}
	}

	return tui.ShortcutBar(shortcuts...)
}

// values copies the entered values back into the options.
func (m *createForm) values(opts *CreateOptions) {
	opts.Name = strings.TrimSpace(m.inputs[fieldName].Value())
	opts.Namespace = strings.TrimSpace(m.inputs[fieldNamespace].Value())

	for _, opt := range m.options {
		switch opt.id {
		case "all-examples":
			opts.AllExamples = opt.checked
		case "console-command":
			opts.ConsoleCommand = opt.checked
		case "scheduled-task":
			opts.ScheduledTask = opt.checked
		case "event-subscriber":
			opts.EventSubscriber = opt.checked
		case "controller":
			opts.Controller = opt.checked
		case "route":
			opts.Route = opt.checked
		case "javascript-plugin":
			opts.JavascriptPlugin = opt.checked
		case "admin-module":
			opts.AdminModule = opt.checked
		case "custom-fieldset":
			opts.CustomFieldset = opt.checked
		}
	}
}

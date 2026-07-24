// Package prompt provides a multi-button confirm overlay for the tui/app
// shell — for questions where a plain yes/no is not enough (e.g. "stop
// containers / quit and keep running / cancel").
//
// The overlay closes itself and emits a ResultMsg; the hosting Content
// handles the outcome:
//
//	case prompt.ResultMsg:
//	    switch msg.Choice { case "stop": ...; case "quit": ... }
package prompt

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// Choice is one button of a prompt.
type Choice struct {
	// ID is returned in ResultMsg.Choice (e.g. "stop", "quit", "cancel").
	ID string
	// Label is the button text.
	Label string
}

// ResultMsg is emitted when the prompt closes. Choice is the chosen
// Choice.ID; empty means the prompt was dismissed with esc.
type ResultMsg struct {
	// ID names the prompt (Options.ID) so apps with several prompts can tell
	// results apart.
	ID     string
	Choice string
}

// Options configure New.
type Options struct {
	// ID names the overlay and is echoed in ResultMsg (default "confirm").
	ID      string
	Title   string
	Message string
	// Choices default to Yes ("yes") / No ("no").
	Choices []Choice
	// Default is the initially selected choice index.
	Default int
	// Danger styles the title with the error color.
	Danger bool
	// MaxWidth bounds the modal (default 70).
	MaxWidth int
}

// Overlay is a centered multi-button confirm modal.
type Overlay struct {
	id       string
	title    string
	message  string
	choices  []Choice
	selected int
	danger   bool
	maxWidth int
}

// New creates a prompt overlay. Push it with Host.PushOverlay.
func New(opts Options) *Overlay {
	if opts.ID == "" {
		opts.ID = "confirm"
	}
	if len(opts.Choices) == 0 {
		opts.Choices = []Choice{{ID: "yes", Label: "Yes"}, {ID: "no", Label: "No"}}
	}
	if opts.MaxWidth <= 0 {
		opts.MaxWidth = 70
	}
	selected := opts.Default
	if selected < 0 || selected >= len(opts.Choices) {
		selected = 0
	}
	return &Overlay{
		id:       opts.ID,
		title:    opts.Title,
		message:  opts.Message,
		choices:  opts.Choices,
		selected: selected,
		danger:   opts.Danger,
		maxWidth: opts.MaxWidth,
	}
}

// Init implements app.Overlay.
func (o *Overlay) Init() tea.Cmd { return nil }

// ID implements app.Overlay.
func (o *Overlay) ID() string { return o.id }

// Update implements app.Overlay.
func (o *Overlay) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return o, nil
	}

	count := len(o.choices)
	switch app.KeyString(key) {
	case tui.KeyLeft, "h":
		if o.selected > 0 {
			o.selected--
		}
	case tui.KeyRight, "l":
		if o.selected < count-1 {
			o.selected++
		}
	case tui.KeyTab:
		o.selected = (o.selected + 1) % count
	case tui.KeyEsc:
		return nil, app.Emit(ResultMsg{ID: o.id})
	case tui.KeyEnter:
		return nil, app.Emit(ResultMsg{ID: o.id, Choice: o.choices[o.selected].ID})
	}
	return o, nil
}

// View implements app.Overlay.
func (o *Overlay) View(width, height int) string {
	modal := tui.NewModal(tui.ModalOptions{MaxWidth: o.maxWidth, AreaWidth: width, AreaHeight: height})

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor)
	if o.danger {
		titleStyle = titleStyle.Foreground(tui.ErrorColor)
	}

	labels := make([]string, len(o.choices))
	for i, c := range o.choices {
		labels[i] = c.Label
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(o.title))
	b.WriteString("\n")
	if o.message != "" {
		b.WriteString(tui.DimStyle.Render(o.message))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(tui.NewButtonRow(tui.ButtonRowOptions{
		Labels:   labels,
		Active:   o.selected,
		MaxWidth: modal.ContentWidth(),
	}).Render())
	b.WriteString("\n\n")
	b.WriteString(tui.ShortcutBar(
		tui.Shortcut{Key: "←/→", Label: "Select"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "esc", Label: "Cancel"},
	))

	return modal.Render(b.String())
}

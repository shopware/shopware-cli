// Package textprompt provides a single-line text-input overlay for the
// tui/app shell — the third of the overlay kits next to prompt (buttons) and
// picker (filterable list).
//
// The overlay closes itself and emits a ResultMsg; the hosting Content
// matches on Key to tell prompts apart:
//
//	case textprompt.ResultMsg:
//	    if field, ok := msg.Key.(configField); ok { ... }
package textprompt

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// ResultMsg is emitted when the prompt closes.
type ResultMsg struct {
	// Key identifies which prompt resolved (Options.Key, may be nil).
	Key       any
	Cancelled bool
	Value     string
}

// Options configure New.
type Options struct {
	// Key is echoed in ResultMsg so apps with several prompts can tell
	// results apart.
	Key   any
	Title string
	// Help is dimmed explanatory text under the title.
	Help string
	// Value pre-fills the input.
	Value string
	// Placeholder defaults to Title.
	Placeholder string
	// Secret masks the typed value (password-style echo).
	Secret bool
	// CharLimit bounds the input length (default 128).
	CharLimit int
	// MaxWidth bounds the modal (default 70).
	MaxWidth int
}

// Overlay is a centered single-line text-input modal.
type Overlay struct {
	key      any
	title    string
	help     string
	maxWidth int
	input    textinput.Model
}

// New creates a text prompt overlay. Push it with Host.PushOverlay.
func New(opts Options) *Overlay {
	if opts.Placeholder == "" {
		opts.Placeholder = opts.Title
	}
	if opts.CharLimit <= 0 {
		opts.CharLimit = 128
	}
	if opts.MaxWidth <= 0 {
		opts.MaxWidth = 70
	}

	ti := textinput.New()
	ti.Placeholder = opts.Placeholder
	ti.CharLimit = opts.CharLimit
	ti.Prompt = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("> ")
	if opts.Secret {
		ti.EchoMode = textinput.EchoPassword
	}
	if opts.Value != "" {
		ti.SetValue(opts.Value)
	}
	ti.Focus()

	return &Overlay{
		key:      opts.Key,
		title:    opts.Title,
		help:     opts.Help,
		maxWidth: opts.MaxWidth,
		input:    ti,
	}
}

// Value returns the current input value.
func (o *Overlay) Value() string { return o.input.Value() }

// Init implements app.Overlay.
func (o *Overlay) Init() tea.Cmd { return textinput.Blink }

// ID implements app.Overlay.
func (o *Overlay) ID() string { return "text-prompt" }

// Update implements app.Overlay.
func (o *Overlay) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return o, nil
	}
	switch app.KeyString(key) {
	case tui.KeyEsc:
		return nil, app.Emit(ResultMsg{Key: o.key, Cancelled: true})
	case tui.KeyEnter:
		return nil, app.Emit(ResultMsg{Key: o.key, Value: o.input.Value()})
	}
	var cmd tea.Cmd
	o.input, cmd = o.input.Update(msg)
	return o, cmd
}

// View implements app.Overlay.
func (o *Overlay) View(width, height int) string {
	modal := tui.NewModal(tui.ModalOptions{MaxWidth: o.maxWidth, AreaWidth: width, AreaHeight: height})

	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor).Render(o.title))
	b.WriteString("\n\n")
	if o.help != "" {
		b.WriteString(tui.DimStyle.Render(o.help))
		b.WriteString("\n\n")
	}
	b.WriteString(o.input.View())
	b.WriteString("\n\n")
	b.WriteString(tui.ShortcutBar(
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "esc", Label: "Cancel"},
	))

	return modal.Render(b.String())
}

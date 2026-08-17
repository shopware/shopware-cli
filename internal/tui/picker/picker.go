// Package picker provides a filterable single-select list overlay for the
// tui/app shell — a type-to-filter input over a windowed list, rendered as a
// centered modal.
//
// The overlay closes itself and emits a ResultMsg; the hosting Content
// matches on Key to tell pickers apart:
//
//	case picker.ResultMsg:
//	    if field, ok := msg.Key.(configField); ok { ... }
package picker

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// Item is one selectable row.
type Item struct {
	// Label is the row text; Detail is dimmed, right-aligned extra text.
	// Both are matched by the filter.
	Label  string
	Detail string
	// Value is returned in ResultMsg.
	Value string
}

// ResultMsg is emitted when the picker closes.
type ResultMsg struct {
	// Key identifies which picker resolved (Options.Key, may be nil).
	Key       any
	Cancelled bool
	Value     string
	// Index is the chosen item's index in Options.Items.
	Index int
}

// Options configure New.
type Options struct {
	// Key is echoed in ResultMsg so apps with several pickers can tell
	// results apart.
	Key   any
	Title string
	// Help is dimmed explanatory text under the title.
	Help  string
	Items []Item
	// InitialIndex preselects an item.
	InitialIndex int
	// Placeholder for the filter input (default "Type to filter").
	Placeholder string
	// Header is an optional column-header line rendered above the rows.
	Header string
	// MaxWidth bounds the modal (default 70).
	MaxWidth int
}

// Overlay is a centered filterable list modal.
type Overlay struct {
	key      any
	title    string
	help     string
	maxWidth int
	list     tui.FilterList
}

// New creates a picker overlay. Push it with Host.PushOverlay.
func New(opts Options) *Overlay {
	if opts.MaxWidth <= 0 {
		opts.MaxWidth = 70
	}

	items := make([]tui.FilterItem, len(opts.Items))
	for i, item := range opts.Items {
		items[i] = tui.FilterItem(item)
	}

	return &Overlay{
		key:      opts.Key,
		title:    opts.Title,
		help:     opts.Help,
		maxWidth: opts.MaxWidth,
		list: tui.NewFilterList(tui.FilterListOptions{
			Items:        items,
			InitialIndex: opts.InitialIndex,
			Placeholder:  opts.Placeholder,
			Header:       opts.Header,
		}),
	}
}

// Len returns the number of items matching the current filter.
func (o *Overlay) Len() int { return o.list.Len() }

// Key returns the identifier the picker was created with.
func (o *Overlay) Key() any { return o.key }

// Items returns all items the picker was created with.
func (o *Overlay) Items() []Item {
	items := make([]Item, len(o.list.Items()))
	for i, item := range o.list.Items() {
		items[i] = Item(item)
	}
	return items
}

// Init implements app.Overlay.
func (o *Overlay) Init() tea.Cmd { return textinput.Blink }

// ID implements app.Overlay.
func (o *Overlay) ID() string { return "picker" }

// Update implements app.Overlay.
func (o *Overlay) Update(msg tea.Msg) (app.Overlay, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch app.KeyString(key) {
		case tui.KeyEsc:
			return nil, app.Emit(ResultMsg{Key: o.key, Cancelled: true})
		case tui.KeyEnter:
			item, index, ok := o.list.Selected()
			if !ok {
				return nil, app.Emit(ResultMsg{Key: o.key, Cancelled: true})
			}
			return nil, app.Emit(ResultMsg{Key: o.key, Value: item.Value, Index: index})
		}
	}

	var cmd tea.Cmd
	o.list, cmd = o.list.Update(msg)
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
	b.WriteString(o.list.View(modal.ContentWidth()))
	b.WriteString("\n\n")
	b.WriteString(tui.ShortcutBar(
		tui.Shortcut{Key: "↑/↓", Label: "Choose"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "esc", Label: "Cancel"},
	))

	return modal.Render(b.String())
}

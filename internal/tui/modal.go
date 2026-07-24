package tui

import (
	"charm.land/lipgloss/v2"
)

// ModalOptions configure a Modal.
type ModalOptions struct {
	// MaxWidth caps the modal box width; the box also never exceeds the area
	// width minus a small margin.
	MaxWidth int
	// AreaWidth and AreaHeight define the region the modal centers within,
	// usually the full terminal or the host's main region.
	AreaWidth  int
	AreaHeight int
}

// Modal is a bordered box centered within an area — the container used by
// overlay dialogs (pickers, detail popups, confirmations).
type Modal struct {
	opts ModalOptions
}

// NewModal creates a modal container.
func NewModal(opts ModalOptions) Modal {
	return Modal{opts: opts}
}

// Width returns the modal box width.
func (m Modal) Width() int {
	width := m.opts.AreaWidth - 4
	if m.opts.MaxWidth > 0 && width > m.opts.MaxWidth {
		width = m.opts.MaxWidth
	}
	if width < 1 {
		width = 1
	}
	return width
}

// ContentWidth returns the columns available to content inside the box
// (border and horizontal padding subtracted). Build content against this
// width before calling Render.
func (m Modal) ContentWidth() int {
	w := m.Width() - 6
	if w < 1 {
		return 1
	}
	return w
}

// Render draws the content inside the centered, bordered box.
func (m Modal) Render(content string) string {
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BrandColor).
		Padding(1, 2).
		Width(m.Width())
	return lipgloss.Place(m.opts.AreaWidth, m.opts.AreaHeight, lipgloss.Center, lipgloss.Center, box.Render(content))
}

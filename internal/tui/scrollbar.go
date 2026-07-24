package tui

import (
	"strings"
)

// ScrollbarOptions configure a Scrollbar.
type ScrollbarOptions struct {
	// Total is the item count; Visible how many fit; Offset the first shown.
	Total   int
	Visible int
	Offset  int
	// Height is the rendered height in rows (arrows included).
	Height int
}

// Scrollbar is a vertical scrollbar column: arrows at both ends, a dashed
// track, and a solid thumb. It renders empty when everything fits.
type Scrollbar struct {
	opts ScrollbarOptions
}

// NewScrollbar creates a scrollbar.
func NewScrollbar(opts ScrollbarOptions) Scrollbar {
	return Scrollbar{opts: opts}
}

// Render implements the component contract.
func (s Scrollbar) Render() string {
	total, visible, offset, height := s.opts.Total, s.opts.Visible, s.opts.Offset, s.opts.Height
	if height < 3 || total <= visible {
		return ""
	}

	track := height - 2
	thumbSize := max(1, track*visible/total)
	maxOffset := total - visible
	thumbPos := 0
	if maxOffset > 0 {
		thumbPos = (track - thumbSize) * offset / maxOffset
	}

	dim := DimStyle
	text := LabelStyle

	var b strings.Builder
	b.WriteString(dim.Render("↑"))
	for i := range track {
		b.WriteString("\n")
		if i >= thumbPos && i < thumbPos+thumbSize {
			b.WriteString(text.Render("█"))
		} else {
			b.WriteString(dim.Render("┆"))
		}
	}
	b.WriteString("\n" + dim.Render("↓"))
	return b.String()
}

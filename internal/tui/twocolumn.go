package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// TwoColumnOptions configure a TwoColumn layout.
type TwoColumnOptions struct {
	// Width is the total width; LeftWidth the columns left of the divider.
	// The divider consumes three columns (" │ ").
	Width     int
	LeftWidth int
	Left      string
	Right     string
}

// TwoColumn renders two blocks side by side separated by a vertical divider,
// top-aligned and padded to equal height. Wizard panels use it for the
// read-only left / user-action right split.
type TwoColumn struct {
	opts TwoColumnOptions
}

// NewTwoColumn creates a two-column layout.
func NewTwoColumn(opts TwoColumnOptions) TwoColumn {
	return TwoColumn{opts: opts}
}

// Render implements the component contract.
func (c TwoColumn) Render() string {
	rightWidth := c.opts.Width - c.opts.LeftWidth - 3
	if rightWidth < 0 {
		rightWidth = 0
	}

	leftLines := splitToWidth(c.opts.Left, c.opts.LeftWidth)
	rightLines := splitToWidth(c.opts.Right, rightWidth)

	rows := max(len(leftLines), len(rightLines))
	divider := lipgloss.NewStyle().Foreground(BorderColor).Render("│")

	var b strings.Builder
	for i := range rows {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		b.WriteString(padToWidth(l, c.opts.LeftWidth))
		b.WriteString(" " + divider + " ")
		b.WriteString(padToWidth(r, rightWidth))
		if i < rows-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// StatusStripOptions configure a StatusStrip.
type StatusStripOptions struct {
	// Variant selects the semantic color of the label.
	Variant Variant
	// Label is the bold colored badge text, e.g. "BLOCKED" or "READY".
	Label string
	// Message is the plain explanation after the label.
	Message string
}

// StatusStrip is a one-line status row: a bold colored label followed by a
// plain message, e.g. "BLOCKED   3 extensions need fixes …".
type StatusStrip struct {
	opts StatusStripOptions
}

// NewStatusStrip creates a status strip.
func NewStatusStrip(opts StatusStripOptions) StatusStrip {
	return StatusStrip{opts: opts}
}

// Render implements the component contract.
func (s StatusStrip) Render() string {
	label := lipgloss.NewStyle().
		Foreground(VariantColor(s.opts.Variant)).
		Bold(true).
		Render(s.opts.Label)

	pad := 10 - lipgloss.Width(s.opts.Label)
	if pad < 2 {
		pad = 2
	}
	return label + strings.Repeat(" ", pad) + LabelStyle.Render(s.opts.Message)
}

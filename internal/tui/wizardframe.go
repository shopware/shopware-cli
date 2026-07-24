package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// WizardFrameOptions configure a WizardFrame.
type WizardFrameOptions struct {
	Width  int
	Height int
	// Title is the panel title in the frame's top row.
	Title string
	// TitleRight is an optional right-aligned label, e.g. "Target 6.7.11.0".
	TitleRight string
	// Status is an optional status row (see StatusStrip) below the title,
	// separated by horizontal rules. Multi-line values render as several rows.
	Status string
	// Body fills the frame's main area, padded or truncated to fit.
	Body string
	// Footer renders at the bottom, separated by a horizontal rule. Empty
	// omits the footer section and lets the body use the full height.
	Footer string
}

// WizardFrame is a bordered wizard panel: title row, optional status row,
// body, and footer, each separated by horizontal rules. It always renders
// exactly Height rows.
type WizardFrame struct {
	opts WizardFrameOptions
}

// NewWizardFrame creates a wizard frame.
func NewWizardFrame(opts WizardFrameOptions) WizardFrame {
	return WizardFrame{opts: opts}
}

// Render implements the component contract.
func (f WizardFrame) Render() string {
	width, height := f.opts.Width, f.opts.Height
	// Clamp against tiny terminals: the borders alone need two columns, and
	// width-2 must never go negative (strings.Repeat panics).
	if width < 4 {
		width = 4
	}
	innerW := width - 4 // "│ " + " │"
	if innerW < 1 {
		innerW = 1
	}

	bc := lipgloss.NewStyle().Foreground(BorderColor)
	top := bc.Render("╭" + strings.Repeat("─", width-2) + "╮")
	bottom := bc.Render("╰" + strings.Repeat("─", width-2) + "╯")
	rule := bc.Render("├" + strings.Repeat("─", width-2) + "┤")

	row := func(content string) string {
		return bc.Render("│") + " " + padToWidth(content, innerW) + " " + bc.Render("│")
	}

	var rows []string
	rows = append(rows, top)
	titleContent := SpreadRow(innerW, TitleStyle.Render(f.opts.Title), DimStyle.Render(f.opts.TitleRight))
	if f.opts.TitleRight == "" {
		titleContent = padToWidth(TitleStyle.Render(f.opts.Title), innerW)
	}
	rows = append(rows, row(titleContent), rule)

	if f.opts.Status != "" {
		for line := range strings.SplitSeq(f.opts.Status, "\n") {
			rows = append(rows, row(line))
		}
		rows = append(rows, rule)
	}

	var footerLines []string
	footerRows := 0
	if f.opts.Footer != "" {
		footerLines = strings.Split(f.opts.Footer, "\n")
		footerRows = len(footerLines) + 1 // rule + footer
	}

	bodyLines := strings.Split(f.opts.Body, "\n")
	bodyHeight := height - len(rows) - footerRows - 1
	for i := range bodyHeight {
		if i < len(bodyLines) {
			rows = append(rows, row(bodyLines[i]))
		} else {
			rows = append(rows, row(""))
		}
	}

	if f.opts.Footer != "" {
		rows = append(rows, rule)
		for _, line := range footerLines {
			rows = append(rows, row(line))
		}
	}
	rows = append(rows, bottom)

	return strings.Join(rows, "\n")
}

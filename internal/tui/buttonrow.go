package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// ButtonRowOptions configure a ButtonRow.
type ButtonRowOptions struct {
	Labels []string
	// Active is the highlighted button index; -1 highlights none.
	Active int
	// MaxWidth wraps buttons onto additional lines when a row would exceed
	// it — for narrow containers such as a panel's user-action column.
	// <= 0 keeps everything on one row.
	MaxWidth int
}

// ButtonRow is a horizontal row of footer buttons with one highlighted.
type ButtonRow struct {
	opts ButtonRowOptions
}

// NewButtonRow creates a button row.
func NewButtonRow(opts ButtonRowOptions) ButtonRow {
	return ButtonRow{opts: opts}
}

// Render implements the component contract.
func (b ButtonRow) Render() string {
	inactiveStyle := lipgloss.NewStyle().
		Foreground(TextColor).
		Background(SubtleBgColor).
		Padding(0, 2)

	buttons := make([]string, len(b.opts.Labels))
	for i, label := range b.opts.Labels {
		if i == b.opts.Active {
			buttons[i] = ActiveButtonStyle.Render(label)
		} else {
			buttons[i] = inactiveStyle.Render(label)
		}
	}

	if b.opts.MaxWidth <= 0 {
		return strings.Join(buttons, "  ")
	}

	// Fill rows left to right, wrapping when the next button would overflow.
	// A blank line between rows keeps wrapped buttons readable as separate
	// buttons instead of one solid block.
	var rows []string
	current := ""
	for _, button := range buttons {
		candidate := button
		if current != "" {
			candidate = current + "  " + button
		}
		if current != "" && lipgloss.Width(candidate) > b.opts.MaxWidth {
			rows = append(rows, current)
			current = button
			continue
		}
		current = candidate
	}
	rows = append(rows, current)
	return strings.Join(rows, "\n\n")
}

// ActiveButtonStyle renders a focused/primary button.
var ActiveButtonStyle = lipgloss.NewStyle().
	Foreground(OnBrandColor).
	Background(BrandColor).
	Bold(true).
	Padding(0, 2)

// ConfirmButtons renders a yes/no button pair with the active side highlighted.
func ConfirmButtons(yesLabel, noLabel string, yesActive bool) string {
	active := 1
	if yesActive {
		active = 0
	}
	return NewButtonRow(ButtonRowOptions{Labels: []string{yesLabel, noLabel}, Active: active}).Render()
}

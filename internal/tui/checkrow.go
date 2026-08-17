package tui

// CheckRowOptions configure a CheckRow.
type CheckRowOptions struct {
	// State drives the leading status dot.
	State DotState
	// Label is the check description; Value the short right column ("yes",
	// "14", …). Value may be pre-styled by the caller.
	Label string
	Value string
	// LabelWidth aligns the value column across rows.
	LabelWidth int
}

// CheckRow is one line of a checklist: a status dot, a padded label, and a
// value column, e.g. "● Git working tree clean    yes".
type CheckRow struct {
	opts CheckRowOptions
}

// NewCheckRow creates a check row.
func NewCheckRow(opts CheckRowOptions) CheckRow {
	return CheckRow{opts: opts}
}

// Render implements the component contract.
func (c CheckRow) Render() string {
	return StateDot(c.opts.State) + " " + LabelStyle.Render(PadRight(c.opts.Label, c.opts.LabelWidth)) + c.opts.Value
}

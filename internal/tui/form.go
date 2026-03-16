package tui

import "charm.land/huh/v2"

const (
	Yes = "yes"
	No  = "no"
)

// NewYesNo returns a huh Select field with Yes/No options.
// The value pointer should be initialized to tui.Yes or tui.No to set the default.
func NewYesNo() *huh.Select[string] {
	return huh.NewSelect[string]().
		Options(
			huh.NewOption("Yes", Yes),
			huh.NewOption("No", No),
		)
}

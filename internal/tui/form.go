package tui

import "charm.land/huh/v2"

// NewYesNo returns a huh Select field with Yes/No options.
// The value pointer should be initialized to "yes" or "no" to set the default.
func NewYesNo() *huh.Select[string] {
	return huh.NewSelect[string]().
		Options(
			huh.NewOption("Yes", "yes"),
			huh.NewOption("No", "no"),
		)
}

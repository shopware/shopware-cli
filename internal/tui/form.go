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

// SetupTheme returns the shared theme for project setup wizards.
func SetupTheme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		s := huh.ThemeCharm(isDark)
		s.Focused.Title = s.Focused.Title.Foreground(BlueColor)
		s.Blurred.Title = s.Blurred.Title.Foreground(BlueColor)
		return s
	})
}

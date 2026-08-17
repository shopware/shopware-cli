package tui

import (
	"charm.land/bubbles/v2/spinner"
	"charm.land/lipgloss/v2"
)

// NewBrandSpinner returns the shared brand-colored dot spinner used by the
// shopware-cli TUIs.
func NewBrandSpinner() spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(BrandColor)),
	)
}

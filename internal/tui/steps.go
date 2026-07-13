package tui

import (
	"charm.land/lipgloss/v2"
)

// fixedIndicator renders an indicator character inside a fixed 2-character-wide column.
func fixedIndicator(indicator string) string {
	return lipgloss.NewStyle().Width(2).Render(indicator)
}

// StepDone renders a completed step with a green checkmark indicator.
func StepDone(label string) string {
	return fixedIndicator(Checkmark) + label + "\n"
}

// StepActive renders an in-progress step with the provided spinner frame.
func StepActive(spinnerView, label string) string {
	return fixedIndicator(spinnerView) + label + "\n"
}

// StepPending renders a step that hasn't started yet with a dimmed dot indicator.
func StepPending(label string) string {
	pending := DimStyle.Render("·")
	return fixedIndicator(pending) + label + "\n"
}

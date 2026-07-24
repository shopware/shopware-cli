package tui

import (
	"charm.land/lipgloss/v2"
)

// DotState is the lifecycle state a StateDot visualizes.
type DotState int

const (
	// DotPending renders a dim "○" for steps that have not started.
	DotPending DotState = iota
	// DotRunning renders a warning-colored "◐" for steps in progress.
	DotRunning
	// DotOK renders a green "●".
	DotOK
	// DotWarn renders a yellow "●".
	DotWarn
	// DotError renders a red "●".
	DotError
)

// StateDot renders the semantic status bullet used in front of checklist and
// queue rows.
func StateDot(s DotState) string {
	switch s {
	case DotOK:
		return lipgloss.NewStyle().Foreground(SuccessColor).Render("●")
	case DotWarn:
		return lipgloss.NewStyle().Foreground(WarnColor).Render("●")
	case DotError:
		return lipgloss.NewStyle().Foreground(ErrorColor).Render("●")
	case DotRunning:
		return lipgloss.NewStyle().Foreground(WarnColor).Render("◐")
	case DotPending:
		return DimStyle.Render("○")
	}
	return DimStyle.Render("○")
}

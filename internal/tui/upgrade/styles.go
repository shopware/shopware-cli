package upgrade

import (
	"charm.land/lipgloss/v2"

	backend "github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui"
)

var (
	// userActionStyle renders the blue "User action" heading of right columns.
	userActionStyle = lipgloss.NewStyle().Foreground(tui.BrandColor).Bold(true)

	okStyle   = lipgloss.NewStyle().Foreground(tui.SuccessColor)
	warnStyle = lipgloss.NewStyle().Foreground(tui.WarnColor)
	failStyle = lipgloss.NewStyle().Foreground(tui.ErrorColor)
)

// dotState maps a backend check state to the shared status dot.
func dotState(s backend.CheckState) tui.DotState {
	switch s {
	case backend.StateOK:
		return tui.DotOK
	case backend.StateWarn:
		return tui.DotWarn
	case backend.StateFail:
		return tui.DotError
	case backend.StateRunning:
		return tui.DotRunning
	case backend.StatePending:
		return tui.DotPending
	}
	return tui.DotPending
}

// stateDot renders the semantic-colored bullet in front of checks and queue rows.
func stateDot(s backend.CheckState) string {
	return tui.StateDot(dotState(s))
}

// statusDot renders the extension status bullet.
func statusDot(s backend.ExtStatus) string {
	switch s {
	case backend.ExtOK:
		return tui.StateDot(tui.DotOK)
	case backend.ExtNeedsUpdate, backend.ExtReview, backend.ExtDeprecated:
		return tui.StateDot(tui.DotWarn)
	case backend.ExtBlocked, backend.ExtMismatch:
		return tui.StateDot(tui.DotError)
	}
	return tui.StateDot(tui.DotPending)
}

// extensionQueueRow renders one extension row of the queue tables: focus
// marker, status dot, name and version-transition columns, result label.
func extensionQueueRow(r backend.ExtensionResult, focused bool, nameWidth, versionWidth int) string {
	cursor := "  "
	if focused {
		cursor = userActionStyle.Render("> ")
	}
	return cursor + statusDot(r.Status) + " " +
		tui.PadRight(tui.Truncate(r.Extension.Name, nameWidth-1), nameWidth) +
		tui.PadRight(versionTransition(r), versionWidth) +
		statusText(r.Status)
}

// statusText renders the extension result column in its semantic color.
func statusText(s backend.ExtStatus) string {
	label := s.Label()
	switch {
	case s == backend.ExtOK:
		return okStyle.Render(label)
	case s.BlocksUpgrade():
		return failStyle.Render(label)
	default:
		return warnStyle.Render(label)
	}
}

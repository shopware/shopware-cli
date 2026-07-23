package upgradetui

import (
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/shop/upgrade"
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
func dotState(s upgrade.CheckState) tui.DotState {
	switch s {
	case upgrade.StateOK:
		return tui.DotOK
	case upgrade.StateWarn:
		return tui.DotWarn
	case upgrade.StateFail:
		return tui.DotError
	case upgrade.StateRunning:
		return tui.DotRunning
	case upgrade.StatePending:
		return tui.DotPending
	}
	return tui.DotPending
}

// stateDot renders the semantic-colored bullet in front of checks and queue rows.
func stateDot(s upgrade.CheckState) string {
	return tui.StateDot(dotState(s))
}

// statusDot renders the extension status bullet.
func statusDot(s upgrade.ExtStatus) string {
	switch s {
	case upgrade.ExtOK:
		return tui.StateDot(tui.DotOK)
	case upgrade.ExtNeedsUpdate, upgrade.ExtReview, upgrade.ExtDeprecated:
		return tui.StateDot(tui.DotWarn)
	case upgrade.ExtBlocked, upgrade.ExtMismatch:
		return tui.StateDot(tui.DotError)
	}
	return tui.StateDot(tui.DotPending)
}

// extensionQueueRow renders one extension row of the queue tables: focus
// marker, status dot, name and version-transition columns, result label.
func extensionQueueRow(r upgrade.ExtensionResult, focused bool, nameWidth, versionWidth int) string {
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
func statusText(s upgrade.ExtStatus) string {
	label := s.Label()
	switch {
	case s == upgrade.ExtOK:
		return okStyle.Render(label)
	case s.BlocksUpgrade():
		return failStyle.Render(label)
	default:
		return warnStyle.Render(label)
	}
}

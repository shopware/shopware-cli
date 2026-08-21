package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// NewUpdateCheckCmd waits for an externally-owned update result and converts
// it into a message understood by Header.Update.
func NewUpdateCheckCmd(wait func(context.Context) bool) tea.Cmd {
	if wait == nil {
		return nil
	}

	return func() tea.Msg {
		if !wait(context.Background()) {
			return nil
		}

		return UpdateAvailableMsg{}
	}
}

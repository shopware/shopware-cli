package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

// NewUpdateCheckCmd waits for an externally-owned update result and converts
// it into a message understood by Header.Update.
func NewUpdateCheckCmd(wait func(context.Context) (bool, error)) tea.Cmd {
	if wait == nil {
		return nil
	}

	return func() tea.Msg {
		available, err := wait(context.Background())
		if err != nil || !available {
			return nil
		}

		return UpdateAvailableMsg{}
	}
}

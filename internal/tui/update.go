package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

type UpdateAvailableMsg struct {
	Version      string
	MarkRendered func() bool
}

// NewUpdateCheckCmd waits for an externally-owned result without coupling the
// TUI package to the update service.
func NewUpdateCheckCmd(wait func(context.Context) (string, bool), markRendered func() bool) tea.Cmd {
	if wait == nil {
		return nil
	}
	return func() tea.Msg {
		version, ok := wait(context.Background())
		if !ok {
			return nil
		}
		return UpdateAvailableMsg{Version: version, MarkRendered: markRendered}
	}
}

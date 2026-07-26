package autofixtui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/shop/pluginmigrate"
)

type scanDoneMsg struct {
	extensions []pluginmigrate.ScannedExtension
}

// availabilityMsg carries what the Composer repositories offer for the
// scanned extensions. With a token the Store catalog is included; the lookup
// against packagist.org and configured repositories always runs.
type availabilityMsg struct {
	token string
	avail pluginmigrate.Availability
	err   error
}

// runEventMsg wraps one runner progress event; runClosedMsg fires when the
// runner's event stream ends.
type runEventMsg pluginmigrate.StepEvent

type runClosedMsg struct{}

func scanCmd(m *pluginmigrate.PluginMigrator) tea.Cmd {
	return func() tea.Msg {
		return scanDoneMsg{extensions: m.Scan(context.Background())}
	}
}

func fetchAvailabilityCmd(m *pluginmigrate.PluginMigrator, token string, extensions []pluginmigrate.ScannedExtension) tea.Cmd {
	return func() tea.Msg {
		avail, err := m.FetchAvailability(context.Background(), token, extensions)
		return availabilityMsg{token: token, avail: avail, err: err}
	}
}

// readRunEventCmd pulls the next runner event; re-issue it after each event.
func readRunEventCmd(events <-chan pluginmigrate.StepEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-events
		if !ok {
			return runClosedMsg{}
		}
		return runEventMsg(ev)
	}
}

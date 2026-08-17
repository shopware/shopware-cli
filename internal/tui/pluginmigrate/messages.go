package pluginmigrate

import (
	"context"

	tea "charm.land/bubbletea/v2"

	migrate "github.com/shopware/shopware-cli/internal/shop/pluginmigrate"
)

type scanDoneMsg struct {
	extensions []migrate.ScannedExtension
}

// availabilityMsg carries what the Composer repositories offer for the
// scanned extensions. With a token the Store catalog is included; the lookup
// against packagist.org and configured repositories always runs.
type availabilityMsg struct {
	token string
	avail migrate.Availability
	err   error
}

// runEventMsg wraps one runner progress event; runClosedMsg fires when the
// runner's event stream ends.
type runEventMsg migrate.StepEvent

type runClosedMsg struct{}

func scanCmd(m *migrate.PluginMigrator) tea.Cmd {
	return func() tea.Msg {
		return scanDoneMsg{extensions: m.Scan(context.Background())}
	}
}

func fetchAvailabilityCmd(m *migrate.PluginMigrator, token string, extensions []migrate.ScannedExtension) tea.Cmd {
	return func() tea.Msg {
		avail, err := m.FetchAvailability(context.Background(), token, extensions)
		return availabilityMsg{token: token, avail: avail, err: err}
	}
}

// readRunEventCmd pulls the next runner event; re-issue it after each event.
func readRunEventCmd(events <-chan migrate.StepEvent) tea.Cmd {
	return func() tea.Msg {
		// Receiving from a nil channel blocks forever and would leak the
		// Bubble Tea command goroutine.
		if events == nil {
			return runClosedMsg{}
		}
		ev, ok := <-events
		if !ok {
			return runClosedMsg{}
		}
		return runEventMsg(ev)
	}
}

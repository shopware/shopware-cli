package devtui

import (
	tea "charm.land/bubbletea/v2"
)

// Modal is a dismissable overlay floating above the dashboard. It owns its own
// state, key handling, and rendering. Returning a nil Modal from Update
// dismisses it. The optional resultCmd is run after dismissal — typically used
// to apply a confirmation result back to the parent Model.
type Modal interface {
	Update(msg tea.Msg) (next Modal, cmd tea.Cmd)
	View(width, height int) string
}

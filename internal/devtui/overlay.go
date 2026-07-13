package devtui

import (
	tea "charm.land/bubbletea/v2"
)

type Modal interface {
	Update(msg tea.Msg) (next Modal, cmd tea.Cmd)
	View(width, height int) string
}

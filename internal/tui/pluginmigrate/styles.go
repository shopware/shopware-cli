package pluginmigrate

import (
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

var (
	okStyle   = lipgloss.NewStyle().Foreground(tui.SuccessColor)
	failStyle = lipgloss.NewStyle().Foreground(tui.ErrorColor)
)

package devtui

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var (
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Border(lipgloss.NormalBorder(), true).
			Padding(0, 1)

	inactiveTabStyle = lipgloss.NewStyle().
				Border(lipgloss.HiddenBorder(), true).
				Padding(0, 1)

	helpStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#9CA3AF"),
		Dark:  lipgloss.Color("#6B7280"),
	})

	labelStyle = lipgloss.NewStyle().Bold(true).Width(20)

	valueStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#047857"),
		Dark:  lipgloss.Color("#04B575"),
	})

	errorStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#DC2626"),
		Dark:  lipgloss.Color("#EF4444"),
	})

	statusStyle = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
		Light: lipgloss.Color("#B8860B"),
		Dark:  lipgloss.Color("#FFD700"),
	}).Bold(true)

	overlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder(), true).
			Padding(1, 3).
			Bold(true)
)

package color

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"
)

var GreenText = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
	Light: lipgloss.Color("#047857"),
	Dark:  lipgloss.Color("#04B575"),
})

var RecommendedText = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
	Light: lipgloss.Color("#047857"),
	Dark:  lipgloss.Color("#04B575"),
}).Bold(true)

var SecondaryText = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
	Light: lipgloss.Color("#B8860B"),
	Dark:  lipgloss.Color("#FFD700"),
}).Bold(true)

var NeutralText = lipgloss.NewStyle().Foreground(compat.AdaptiveColor{
	Light: lipgloss.Color("#1F2937"),
	Dark:  lipgloss.Color("#FFFFFF"),
})

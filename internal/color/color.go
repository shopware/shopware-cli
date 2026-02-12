package color

import "github.com/charmbracelet/lipgloss"

var GreenText = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#047857",
	Dark:  "#04B575",
})

var RecommendedText = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#047857",
	Dark:  "#04B575",
}).Bold(true)

var SecondaryText = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#B8860B",
	Dark:  "#FFD700",
}).Bold(true)

var NeutralText = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#1F2937",
	Dark:  "#FFFFFF",
})

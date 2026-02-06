package color

import "github.com/charmbracelet/lipgloss"

// AdaptiveColor picks light/dark variants automatically based on terminal background
var GreenText = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#047857", // darker green for light backgrounds
	Dark:  "#04B575", // lighter green for dark backgrounds
})

var RecommendedText = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#047857",
	Dark:  "#04B575",
}).Bold(true)

var SecondaryText = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#B8860B", // darker gold for light backgrounds
	Dark:  "#FFD700",
}).Bold(true)

var NeutralText = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
	Light: "#1F2937", // dark gray for light backgrounds
	Dark:  "#FFFFFF",
})

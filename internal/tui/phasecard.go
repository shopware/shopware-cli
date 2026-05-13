package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const PhaseCardWidth = 79

// RenderPhaseCard renders content inside a fixed-width card with the Shopware
// mascot art at the top, a divider, and the content below.
func RenderPhaseCard(content string) string {
	return renderPhaseCard(MascotStandard(PhaseCardWidth-2), content)
}

// RenderPhaseCardCowsay renders content inside a fixed-width card with the
// Shopware mascot saying the given text via a speech bubble.
func RenderPhaseCardCowsay(speech, content string) string {
	return renderPhaseCard(MascotCowsay(PhaseCardWidth-2, speech), content)
}

func renderPhaseCard(logo, content string) string {
	innerW := PhaseCardWidth - 2

	logoSection := lipgloss.NewStyle().
		Width(innerW).
		Render(logo)

	divider := lipgloss.NewStyle().Foreground(BorderColor).Render(strings.Repeat("─", innerW))

	contentSection := lipgloss.NewStyle().
		Width(innerW).
		Padding(1, 3).
		Render(content)

	inner := lipgloss.JoinVertical(lipgloss.Left, logoSection, divider, contentSection)

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderColor).
		Width(PhaseCardWidth).
		Render(inner)
}

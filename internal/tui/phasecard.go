package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const PhaseCardWidth = 79

// minMascotCardWidth is the narrowest card that still shows the mascot art.
const minMascotCardWidth = 32

// phaseCardChromeRows are the border and padding rows around the card content.
const phaseCardChromeRows = 4

// RenderPhaseCard renders content inside a fixed-width card with the Shopware
// mascot art at the top, a divider, and the content below.
func RenderPhaseCard(content string) string {
	return RenderPhaseCardFit(content, 0, 0)
}

// RenderPhaseCardCowsay renders content inside a fixed-width card with the
// Shopware mascot saying the given text via a speech bubble.
func RenderPhaseCardCowsay(speech, content string) string {
	return RenderPhaseCardCowsayFit(speech, content, 0, 0)
}

// RenderPhaseCardFit renders a phase card within maxWidth x maxHeight, dropping the mascot when it would not fit; 0 means unbounded.
func RenderPhaseCardFit(content string, maxWidth, maxHeight int) string {
	return fitPhaseCard(MascotStandard, content, maxWidth, maxHeight)
}

// RenderPhaseCardCowsayFit is RenderPhaseCardFit with the speaking mascot.
func RenderPhaseCardCowsayFit(speech, content string, maxWidth, maxHeight int) string {
	return fitPhaseCard(func(width int) string { return MascotCowsay(width, speech) }, content, maxWidth, maxHeight)
}

// PhaseCardContentRows returns the content rows a card without the mascot has within maxHeight; 0 means unbounded.
func PhaseCardContentRows(maxHeight int) int {
	if maxHeight <= 0 {
		return 0
	}
	return max(maxHeight-phaseCardChromeRows, 1)
}

func fitPhaseCard(mascot func(width int) string, content string, maxWidth, maxHeight int) string {
	width := PhaseCardWidth
	if maxWidth > 0 && maxWidth < width {
		width = maxWidth
	}

	if width >= minMascotCardWidth {
		card := renderPhaseCard(mascot(width-2), content, width)
		if maxHeight <= 0 || lipgloss.Height(card) <= maxHeight {
			return card
		}
	}

	return renderPhaseCard("", content, width)
}

func renderPhaseCard(logo, content string, width int) string {
	innerW := width - 2

	inner := lipgloss.NewStyle().
		Width(innerW).
		Padding(1, 3).
		Render(content)

	if logo != "" {
		logoSection := lipgloss.NewStyle().
			Width(innerW).
			Render(logo)

		divider := lipgloss.NewStyle().Foreground(BorderColor).Render(strings.Repeat("─", innerW))

		inner = lipgloss.JoinVertical(lipgloss.Left, logoSection, divider, inner)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(BorderColor).
		Width(width).
		Render(inner)
}

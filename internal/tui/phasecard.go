package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const mascotArt = `____________________________
____________________________
____________▓▓▓▓██████______
______████▓▓▓▓▓▓████████____
____██████▓▓▓▓▓▓████████____
__████████▓▓▓▓▓▓██▒█████____
▓▓██████████▓▓▓▓████████____
__██████████████████████____
__████████████████__██████__
__██████____██████______████
__▓▓▓▓▓▓____▓▓▓▓▓▓__________
____________________________
____________________________`

const PhaseCardWidth = 79

func renderMascotArt(targetWidth int) string {
	const dashChar = '·'

	dotStyle := lipgloss.NewStyle().Foreground(BorderColor)
	bodyStyle := lipgloss.NewStyle().Foreground(BrandColor)
	shadeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#0450A0"))
	eyeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFFFFF"))

	classOf := func(ch rune) int {
		switch ch {
		case dashChar:
			return 0
		case '▓':
			return 1
		case '▒':
			return 2
		default:
			return 3
		}
	}

	styleOf := func(ch rune) lipgloss.Style {
		switch classOf(ch) {
		case 0:
			return dotStyle
		case 1:
			return shadeStyle
		case 2:
			return eyeStyle
		default:
			return bodyStyle
		}
	}

	lines := strings.Split(mascotArt, "\n")

	maxW := 0
	for _, line := range lines {
		if w := len([]rune(line)); w > maxW {
			maxW = w
		}
	}

	padTotal := targetWidth - maxW
	leftPad, rightPad := 0, 0
	if padTotal > 0 {
		leftPad = padTotal / 2
		rightPad = padTotal - leftPad
	}

	var result strings.Builder
	for i, line := range lines {
		runes := []rune(strings.ReplaceAll(line, "_", string(dashChar)))
		for len(runes) < maxW {
			runes = append(runes, dashChar)
		}
		if leftPad > 0 {
			pad := make([]rune, 0, leftPad+len(runes))
			for j := 0; j < leftPad; j++ {
				pad = append(pad, dashChar)
			}
			runes = append(pad, runes...)
		}
		for j := 0; j < rightPad; j++ {
			runes = append(runes, dashChar)
		}

		var batch []rune
		curCls := classOf(runes[0])
		for _, ch := range runes {
			cls := classOf(ch)
			if cls != curCls && len(batch) > 0 {
				result.WriteString(styleOf(batch[0]).Render(string(batch)))
				batch = batch[:0]
			}
			curCls = cls
			batch = append(batch, ch)
		}
		if len(batch) > 0 {
			result.WriteString(styleOf(batch[0]).Render(string(batch)))
		}
		if i < len(lines)-1 {
			result.WriteByte('\n')
		}
	}
	return result.String()
}

// RenderPhaseCard renders content inside a fixed-width card with the Shopware
// mascot art at the top, a divider, and the content below.
func RenderPhaseCard(content string) string {
	innerW := PhaseCardWidth - 2

	logoSection := lipgloss.NewStyle().
		Width(innerW).
		Render(renderMascotArt(innerW))

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

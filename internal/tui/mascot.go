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

// mascotVisibleWidth is the rune-width of a single mascot art line.
// All lines in mascotArt are normalized to this width.
const mascotVisibleWidth = 28

// mascotHeadCol is the column (0-indexed, within the 28-col art) where the
// mascot's head is roughly centered. Used to align speech-bubble tails.
const mascotHeadCol = 16

// MascotStandard renders the Shopware mascot, horizontally centered within
// targetWidth columns using dot padding.
func MascotStandard(targetWidth int) string {
	return renderMascot(targetWidth)
}

// MascotCowsay renders the Shopware mascot with a speech bubble above it
// containing the given text. The bubble is centered above the mascot and
// sized to fit text (wrapping long lines to targetWidth-4).
func MascotCowsay(targetWidth int, text string) string {
	mascot := trimLeadingBlankRows(renderMascot(targetWidth))
	bubble := renderSpeechBubble(text, targetWidth)
	return bubble + "\n" + mascot
}

// trimLeadingBlankRows removes leading rows from rendered art that contain
// no body pixels (only dot-padding or whitespace). Used to tighten the gap
// between a speech bubble and the mascot's head.
func trimLeadingBlankRows(art string) string {
	lines := strings.Split(art, "\n")
	i := 0
	for i < len(lines) && isBlankRow(lines[i]) {
		i++
	}
	return strings.Join(lines[i:], "\n")
}

func isBlankRow(line string) bool {
	for _, r := range stripANSI(line) {
		if r != '·' && r != ' ' {
			return false
		}
	}
	return true
}

// stripANSI removes ANSI SGR escape sequences so we can inspect the raw
// glyphs of a styled string.
func stripANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == 0x1b {
			in = true
			continue
		}
		if in {
			if r == 'm' {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func renderMascot(targetWidth int) string {
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

// renderSpeechBubble draws a rounded-border bubble with a downward tail
// pointing at the mascot's head, centered horizontally within targetWidth.
func renderSpeechBubble(text string, targetWidth int) string {
	bubbleStyle := lipgloss.NewStyle().Foreground(BrandColor)
	dotStyle := lipgloss.NewStyle().Foreground(BorderColor)

	// Maximum text width inside the bubble: leave 4 cols for borders+padding,
	// and clamp to a sensible reading width.
	maxTextWidth := min(targetWidth-4, 60)
	maxTextWidth = max(maxTextWidth, 8)

	lines := wrapText(text, maxTextWidth)

	contentW := 0
	for _, line := range lines {
		contentW = max(contentW, lipgloss.Width(line))
	}
	innerW := contentW + 2 // 1 col padding either side

	// Mascot's head center sits at column leftPad + mascotVisibleWidth/2
	// (relative to a targetWidth-wide canvas). Position the bubble so its
	// tail lines up under that column.
	mascotLeftPad := max((targetWidth-mascotVisibleWidth)/2, 0)
	headCenter := mascotLeftPad + mascotHeadCol

	// Bubble width is innerW + 2 (borders). Try to center the bubble's tail
	// on headCenter, but clamp so the bubble stays within targetWidth.
	bubbleW := innerW + 2
	bubbleLeft := max(headCenter-bubbleW/2, 0)
	if bubbleLeft+bubbleW > targetWidth {
		bubbleLeft = targetWidth - bubbleW
		if bubbleLeft < 0 {
			bubbleLeft = 0
			bubbleW = targetWidth
			innerW = bubbleW - 2
		}
	}

	tailCol := max(headCenter-bubbleLeft, 1) // column within the bubble where the tail lives
	tailCol = min(tailCol, bubbleW-2)

	leftDots := strings.Repeat("·", bubbleLeft)
	rightDots := func(used int) string {
		n := max(targetWidth-bubbleLeft-used, 0)
		return strings.Repeat("·", n)
	}

	var b strings.Builder

	// Top border: ╭─────╮
	top := "╭" + strings.Repeat("─", innerW) + "╮"
	b.WriteString(dotStyle.Render(leftDots))
	b.WriteString(bubbleStyle.Render(top))
	b.WriteString(dotStyle.Render(rightDots(lipgloss.Width(top))))
	b.WriteByte('\n')

	// Text rows: │ text │
	for _, line := range lines {
		pad := contentW - lipgloss.Width(line)
		row := "│ " + line + strings.Repeat(" ", pad) + " │"
		b.WriteString(dotStyle.Render(leftDots))
		b.WriteString(bubbleStyle.Render(row))
		b.WriteString(dotStyle.Render(rightDots(lipgloss.Width(row))))
		b.WriteByte('\n')
	}

	// Bottom border: ╰─────╯ (clean, no tail notch)
	bottom := "╰" + strings.Repeat("─", innerW) + "╯"
	b.WriteString(dotStyle.Render(leftDots))
	b.WriteString(bubbleStyle.Render(bottom))
	b.WriteString(dotStyle.Render(rightDots(lipgloss.Width(bottom))))
	b.WriteByte('\n')

	// Tail: a "\/" shape below the bubble centered on tailCol
	tailLeftWidth := max(bubbleLeft+tailCol-1, 0)
	tailLeft := strings.Repeat("·", tailLeftWidth)
	tailRight := strings.Repeat("·", targetWidth-tailLeftWidth-2)
	b.WriteString(dotStyle.Render(tailLeft))
	b.WriteString(bubbleStyle.Render(`\/`))
	b.WriteString(dotStyle.Render(tailRight))

	return b.String()
}

// wrapText breaks text into lines no wider than maxWidth, preserving
// explicit newlines and breaking on word boundaries where possible.
func wrapText(text string, maxWidth int) []string {
	if maxWidth < 1 {
		maxWidth = 1
	}
	var out []string
	for paragraph := range strings.SplitSeq(text, "\n") {
		if paragraph == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, w := range words {
			switch {
			case line == "":
				line = w
			case lipgloss.Width(line)+1+lipgloss.Width(w) <= maxWidth:
				line += " " + w
			default:
				out = append(out, line)
				line = w
			}
			// hard-break very long words
			for lipgloss.Width(line) > maxWidth {
				runes := []rune(line)
				out = append(out, string(runes[:maxWidth]))
				line = string(runes[maxWidth:])
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		out = []string{""}
	}
	return out
}

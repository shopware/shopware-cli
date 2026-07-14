package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// RenderTwoColumns lays out left and right content side by side within the
// given total width, separated by a vertical rule, at least minHeight rows
// tall (pass 0 to just grow to the taller column). rightFraction is the
// share of width given to the right column (e.g. 0.38); the left column
// gets the rest, minus the 3-column gutter (" │ "). Renders no border or
// title of its own.
func RenderTwoColumns(width, minHeight int, rightFraction float64, left, right string) string {
	rightW := int(float64(width) * rightFraction)
	leftW := width - rightW - 3 // 3 = " │ " gutter + divider

	leftCol := lipgloss.NewStyle().Width(leftW).Render(left)
	rightCol := lipgloss.NewStyle().Width(rightW).Render(right)

	rows := max(minHeight, lipgloss.Height(leftCol), lipgloss.Height(rightCol))
	leftCol = lipgloss.NewStyle().Width(leftW).Height(rows).Render(left)
	rightCol = lipgloss.NewStyle().Width(rightW).Height(rows).Render(right)

	divider := lipgloss.NewStyle().Foreground(BorderColor).
		Render(strings.TrimSuffix(strings.Repeat("│\n", rows), "\n"))
	gutter := lipgloss.NewStyle().Width(1).Height(rows).Render("")

	return lipgloss.JoinHorizontal(lipgloss.Top, leftCol, gutter, divider, gutter, rightCol)
}

package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// SpreadRow places left and right on one row, padding the middle with spaces.
func SpreadRow(width int, left, right string) string {
	fill := width - lipgloss.Width(left) - lipgloss.Width(right)
	if fill < 1 {
		fill = 1
	}
	return left + strings.Repeat(" ", fill) + right
}

// Truncate shortens s to at most width terminal columns, appending an
// ellipsis when it cuts. ANSI sequences and wide runes are handled correctly.
func Truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width <= 1 {
		return "…"
	}
	return ansi.Truncate(s, width, "…")
}

// PadRight pads s with spaces to the given number of terminal columns. Longer
// strings are returned with a single trailing space instead of truncating —
// use Truncate first when a hard column limit is needed.
func PadRight(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s + " "
	}
	return s + strings.Repeat(" ", width-w)
}

// JoinColumns glues two multi-line strings side by side with a gap,
// top-aligned; the left block is padded to its widest line.
func JoinColumns(left, right string, gap int) string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")

	width := 0
	for _, l := range leftLines {
		width = max(width, lipgloss.Width(l))
	}

	var b strings.Builder
	rows := max(len(leftLines), len(rightLines))
	for i := range rows {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		b.WriteString(l + strings.Repeat(" ", width-lipgloss.Width(l)+gap) + r)
		if i < rows-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// splitToWidth splits s into lines, truncating each to the given width.
func splitToWidth(s string, width int) []string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if lipgloss.Width(line) > width {
			lines[i] = ansi.Truncate(line, width, "…")
		}
	}
	return lines
}

// padToWidth pads s with spaces to exactly width terminal columns,
// truncating when it is too long.
func padToWidth(s string, width int) string {
	w := lipgloss.Width(s)
	if w > width {
		return ansi.Truncate(s, width, "…")
	}
	return s + strings.Repeat(" ", width-w)
}

package tui

import "charm.land/lipgloss/v2"

// AppendTail appends more to lines, dropping the oldest entries once the
// buffer exceeds keep — the shared scrollback shape of streamed command
// output. keep <= 0 keeps everything.
func AppendTail(lines []string, keep int, more ...string) []string {
	lines = append(lines, more...)
	if keep > 0 && len(lines) > keep {
		lines = lines[len(lines)-keep:]
	}
	return lines
}

// TailLines returns the last n lines (all of them when n >= len). n <= 0
// returns nil.
func TailLines(lines []string, n int) []string {
	if n <= 0 {
		return nil
	}
	if len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

// TailWrappedLines returns the trailing lines that fit into maxRows once
// wrapping at width is taken into account. Long command output wraps onto
// several rows, so tailing by slice element alone overflows a fixed-height
// panel and clips whatever sits at the end. The last line is always included,
// even when it alone exceeds maxRows.
func TailWrappedLines(lines []string, maxRows, width int) []string {
	if maxRows <= 0 || len(lines) == 0 {
		return nil
	}

	rows := 0
	start := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		cost := wrappedRows(lines[i], width)
		if rows+cost > maxRows && i < len(lines)-1 {
			break
		}
		rows += cost
		start = i
	}
	return lines[start:]
}

// wrappedRows reports how many terminal rows a line occupies at the given
// width. A width of 0 or less means no wrapping.
func wrappedRows(line string, width int) int {
	if width <= 0 {
		return 1
	}
	w := lipgloss.Width(line)
	if w <= width {
		return 1
	}
	return (w + width - 1) / width
}

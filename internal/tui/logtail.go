package tui

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

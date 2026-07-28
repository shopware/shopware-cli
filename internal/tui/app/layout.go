package app

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Region sizes for a frame layout.
type Region struct {
	Header int
	Main   int
	Footer int
	Width  int
	Height int
}

// ComputeRegion allocates vertical space: header and footer keep their natural
// height (clamped), main gets the remainder.
func ComputeRegion(width, height int, header, footer string) Region {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	hh := 0
	if header != "" {
		hh = lipgloss.Height(header)
	}
	fh := 0
	if footer != "" {
		fh = lipgloss.Height(footer)
	}

	// Clamp chrome so main can exist: shrink footer first, then header.
	for fh > 0 && hh+fh >= height {
		fh--
	}
	for hh > 0 && hh+fh >= height {
		hh--
	}

	main := height - hh - fh
	if main < 0 {
		main = 0
	}
	return Region{Header: hh, Main: main, Footer: fh, Width: width, Height: height}
}

// Frame stacks header, main, and footer into a fixed terminal height. Main is
// padded or truncated to fill the middle region.
func Frame(width, height int, header, main, footer string) string {
	r := ComputeRegion(width, height, header, footer)
	parts := make([]string, 0, 3)
	if r.Header > 0 {
		parts = append(parts, fitHeight(header, r.Header, r.Width))
	}
	parts = append(parts, fitHeight(main, r.Main, r.Width))
	if r.Footer > 0 {
		parts = append(parts, fitHeight(footer, r.Footer, r.Width))
	}
	return strings.Join(parts, "\n")
}

// JoinHorizontal joins blocks side by side with a gap, top-aligned.
func JoinHorizontal(gap int, blocks ...string) string {
	if gap < 0 {
		gap = 0
	}
	if len(blocks) == 0 {
		return ""
	}
	sep := strings.Repeat(" ", gap)
	return lipgloss.JoinHorizontal(lipgloss.Top, intersperse(blocks, sep)...)
}

func intersperse(blocks []string, sep string) []string {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]string, 0, len(blocks)*2-1)
	for i, b := range blocks {
		if i > 0 {
			out = append(out, sep)
		}
		out = append(out, b)
	}
	return out
}

// fitHeight pads or crops content to exactly h lines, truncating each line to
// width columns.
func fitHeight(content string, h, width int) string {
	if h <= 0 {
		return ""
	}
	var lines []string
	if content != "" {
		lines = strings.Split(content, "\n")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	if width > 0 {
		for i, line := range lines {
			if lipgloss.Width(line) > width {
				lines[i] = ansi.Truncate(line, width, "")
			}
		}
	}
	return strings.Join(lines, "\n")
}

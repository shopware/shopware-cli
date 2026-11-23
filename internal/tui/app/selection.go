package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

// A terminal cannot select text while the app captures the mouse, so the shell
// selects itself: dragging with the left button marks a block of cells and
// releasing copies it. The selection is a block rather than a text flow so a
// drag inside one pane does not pick up the panes and borders beside it.
// Not part of lattice's shell.

type cell struct{ x, y int }

type selection struct {
	anchor, cursor cell
	dragging       bool
	visible        bool
}

// bounds returns the selected block as inclusive cell coordinates.
func (s selection) bounds() (left, top, right, bottom int) {
	return min(s.anchor.x, s.cursor.x), min(s.anchor.y, s.cursor.y),
		max(s.anchor.x, s.cursor.x), max(s.anchor.y, s.cursor.y)
}

// updateSelection tracks a drag and reports whether it consumed the message.
// The wheel is never consumed, so content keeps scrolling.
func (a *App) updateSelection(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		a.selection = selection{}
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			return nil, false
		}
		at := cell{msg.X, msg.Y}
		a.selection = selection{anchor: at, cursor: at, dragging: true}
		return nil, true
	case tea.MouseMotionMsg:
		if !a.selection.dragging {
			return nil, false
		}
		a.selection.cursor = cell{msg.X, msg.Y}
		a.selection.visible = a.selection.cursor != a.selection.anchor
		return nil, true
	case tea.MouseReleaseMsg:
		if !a.selection.dragging {
			return nil, false
		}
		a.selection.dragging = false
		if !a.selection.visible {
			return nil, true
		}
		text := selectedText(a.frame, a.selection)
		if text == "" {
			a.selection = selection{}
			return nil, true
		}
		return copyToClipboard(text), true
	}
	return nil, false
}

// copyToClipboard writes through the terminal (OSC 52, which also works over
// SSH) and through the system clipboard, because not every terminal supports
// the former and not every session has the latter.
func copyToClipboard(text string) tea.Cmd {
	return tea.Batch(tea.SetClipboard(text), func() tea.Msg {
		_ = clipboard.WriteAll(text)
		return nil
	})
}

// selectedText returns the plain text of the selected block of a rendered frame.
func selectedText(frame string, s selection) string {
	left, top, right, bottom := s.bounds()
	lines := strings.Split(frame, "\n")

	var rows []string
	for y := max(top, 0); y <= bottom && y < len(lines); y++ {
		row := ansi.Strip(ansi.Cut(lines[y], left, right+1))
		rows = append(rows, strings.TrimRight(row, " "))
	}
	return strings.Trim(strings.Join(rows, "\n"), "\n")
}

var selectionStyle = lipgloss.NewStyle().Reverse(true)

// highlightSelection marks the selected block in a rendered frame.
func highlightSelection(frame string, s selection) string {
	left, top, right, bottom := s.bounds()
	lines := strings.Split(frame, "\n")

	for y := max(top, 0); y <= bottom && y < len(lines); y++ {
		width := ansi.StringWidth(lines[y])
		end := min(right+1, width)
		if left >= end {
			continue
		}
		lines[y] = ansi.Cut(lines[y], 0, left) +
			selectionStyle.Render(ansi.Strip(ansi.Cut(lines[y], left, end))) +
			ansi.Cut(lines[y], end, width)
	}
	return strings.Join(lines, "\n")
}

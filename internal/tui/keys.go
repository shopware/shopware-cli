package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// String forms of the special keys as produced by KeyString. Printable keys
// ("q", "j", "1", …) are matched with plain literals instead.
const (
	KeyCtrlC    = "ctrl+c"
	KeyEnter    = "enter"
	KeyEsc      = "esc"
	KeyUp       = "up"
	KeyDown     = "down"
	KeyLeft     = "left"
	KeyRight    = "right"
	KeyTab      = "tab"
	KeyShiftTab = "shift+tab"
)

// KeyString returns the string form of a key press, lower-cased. With Caps
// Lock on, the terminal sends the shortcut's shifted form instead of the
// plain one - e.g. pressing ctrl+c sends "ctrl+C" and ctrl+p sends "ctrl+P" -
// which doesn't match lowercase key constants ("ctrl+c", "ctrl+p", ...), so
// the shortcut silently does nothing. Lower-casing here makes the match work
// either way.
func KeyString(msg tea.KeyPressMsg) string {
	return strings.ToLower(msg.String())
}

// MoveCursor applies an up/down (or k/j) navigation key to a cursor over a
// list of length count, clamping to [0, count-1]. Keys other than the four
// navigation keys leave the cursor unchanged.
func MoveCursor(cursor int, key string, count int) int {
	switch key {
	case KeyUp, "k":
		if cursor > 0 {
			return cursor - 1
		}
	case KeyDown, "j":
		if cursor < count-1 {
			return cursor + 1
		}
	}
	return cursor
}

// ConfirmNav applies the shared yes/no confirm-row navigation: left (or h)
// picks yes, right (or l) picks no, tab toggles. Other keys leave the
// selection unchanged.
func ConfirmNav(confirmYes bool, key string) bool {
	switch key {
	case KeyLeft, "h":
		return true
	case KeyRight, "l":
		return false
	case KeyTab:
		return !confirmYes
	}
	return confirmYes
}

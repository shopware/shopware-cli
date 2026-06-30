package devtui

const (
	keyCtrlC    = "ctrl+c"
	keyDown     = "down"
	keyEnter    = "enter"
	keyUp       = "up"
	keyTab      = "tab"
	keyShiftTab = "shift+tab"
	keyQ        = "q"
	keyF        = "f"
	keyJ        = "j"
	keyK        = "k"
	key1        = "1"
	key2        = "2"
	key3        = "3"
	keyLeft     = "left"
	keyRight    = "right"
)

// moveCursor applies an up/down (or k/j) navigation key to a cursor over a
// list of length count, clamping to [0, count-1]. Keys other than the four
// navigation keys leave the cursor unchanged. Shared by the wizard steps that
// pick a single value from a fixed slice.
func moveCursor(cursor int, key string, count int) int {
	switch key {
	case keyUp, keyK:
		if cursor > 0 {
			return cursor - 1
		}
	case keyDown, keyJ:
		if cursor < count-1 {
			return cursor + 1
		}
	}
	return cursor
}

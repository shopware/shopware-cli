package tui

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestAppendTail(t *testing.T) {
	lines := AppendTail(nil, 3, "a", "b")
	assert.Equal(t, []string{"a", "b"}, lines)

	lines = AppendTail(lines, 3, "c", "d")
	assert.Equal(t, []string{"b", "c", "d"}, lines, "oldest lines are dropped past keep")

	lines = AppendTail([]string{"a", "b"}, 0, "c")
	assert.Equal(t, []string{"a", "b", "c"}, lines, "keep <= 0 keeps everything")
}

func TestTailLines(t *testing.T) {
	lines := []string{"a", "b", "c"}

	assert.Equal(t, []string{"b", "c"}, TailLines(lines, 2))
	assert.Equal(t, lines, TailLines(lines, 5), "n past length returns all lines")
	assert.Nil(t, TailLines(lines, 0))
}

func TestConfirmNav(t *testing.T) {
	assert.True(t, ConfirmNav(false, KeyLeft), "left picks yes")
	assert.True(t, ConfirmNav(false, "h"))
	assert.False(t, ConfirmNav(true, KeyRight), "right picks no")
	assert.False(t, ConfirmNav(true, "l"))
	assert.True(t, ConfirmNav(false, KeyTab), "tab toggles")
	assert.False(t, ConfirmNav(true, KeyTab))
	assert.True(t, ConfirmNav(true, KeyEnter), "other keys leave the selection")
}

func TestCheckbox(t *testing.T) {
	assert.Equal(t, "[x] Show password", ansi.Strip(Checkbox(true, false, "Show password")))
	assert.Equal(t, "[ ] Show password", ansi.Strip(Checkbox(false, true, "Show password")))
}

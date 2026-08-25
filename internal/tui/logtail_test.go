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

func TestTailWrappedLines(t *testing.T) {
	short := []string{"a", "b", "c"}
	assert.Equal(t, []string{"b", "c"}, TailWrappedLines(short, 2, 10))
	assert.Equal(t, short, TailWrappedLines(short, 5, 10), "budget past length returns all lines")
	assert.Nil(t, TailWrappedLines(short, 0, 10))
	assert.Nil(t, TailWrappedLines(nil, 3, 10))

	// "aaaaaaaaaa" occupies two rows at width 5, so it costs twice as much as
	// a short line and pushes the older lines out of the budget.
	wrapping := []string{"x", "y", "aaaaaaaaaa"}
	assert.Equal(t, []string{"y", "aaaaaaaaaa"}, TailWrappedLines(wrapping, 3, 5))
	assert.Equal(t, []string{"aaaaaaaaaa"}, TailWrappedLines(wrapping, 2, 5))

	assert.Equal(t, []string{"aaaaaaaaaa"}, TailWrappedLines(wrapping, 1, 5),
		"the last line is kept even when it alone exceeds the budget")

	assert.Equal(t, short, TailWrappedLines(short, 3, 0), "width <= 0 disables wrapping")
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

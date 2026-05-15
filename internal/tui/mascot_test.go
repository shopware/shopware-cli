package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestMascotStandardWidth(t *testing.T) {
	out := MascotStandard(40)
	for i, line := range strings.Split(out, "\n") {
		assert.Equal(t, 40, lipgloss.Width(line), "line %d should be 40 cols", i)
	}
}

func TestMascotCowsayContainsBubbleAndMascot(t *testing.T) {
	out := MascotCowsay(60, "Hello there")
	// Bubble border characters present
	assert.Contains(t, out, "╭")
	assert.Contains(t, out, "╮")
	assert.Contains(t, out, "╰")
	assert.Contains(t, out, "╯")
	assert.Contains(t, out, "│")
	assert.Contains(t, out, `\/`) // tail
	// Speech text present
	assert.Contains(t, out, "Hello there")
	// Mascot art present
	assert.Contains(t, out, "▓")
}

func TestMascotCowsayLineWidthsMatchTarget(t *testing.T) {
	out := MascotCowsay(60, "Welcome to Shopware")
	for i, line := range strings.Split(out, "\n") {
		assert.Equal(t, 60, lipgloss.Width(line), "line %d should be 60 cols, got %q", i, line)
	}
}

func TestMascotCowsayWrapsLongText(t *testing.T) {
	short := MascotCowsay(60, "Hi")
	long := "This is a really long message that should definitely wrap onto multiple lines inside the speech bubble."
	longOut := MascotCowsay(60, long)
	// Wrapping must add at least one extra content row to the bubble.
	assert.Greater(t, strings.Count(longOut, "\n"), strings.Count(short, "\n"))
	// Original message words still present somewhere
	assert.Contains(t, longOut, "really long message")
}

func TestWrapText(t *testing.T) {
	got := wrapText("hello world this is a test", 11)
	assert.Equal(t, []string{"hello world", "this is a", "test"}, got)
}

func TestWrapTextHardBreaksLongWord(t *testing.T) {
	got := wrapText("supercalifragilistic", 6)
	assert.Equal(t, []string{"superc", "alifra", "gilist", "ic"}, got)
}

func TestWrapTextPreservesExplicitNewlines(t *testing.T) {
	got := wrapText("first\nsecond line here", 100)
	assert.Equal(t, []string{"first", "second line here"}, got)
}

package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeOptions(n int) []SelectOption {
	opts := make([]SelectOption, n)
	for i := range opts {
		opts[i] = SelectOption{Label: "v" + strconv.Itoa(i)}
	}
	return opts
}

// countOptionRows counts rendered option lines (each shows a "vN" label),
// ignoring the title/description/scroll-indicator lines.
func countOptionRows(out string) int {
	n := 0
	for _, line := range strings.Split(stripANSI(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "v") || strings.HasPrefix(trimmed, "● v") {
			n++
		}
	}
	return n
}

func TestRenderSelectListWindowedCapsVisibleRows(t *testing.T) {
	t.Parallel()

	out := stripANSI(RenderSelectListWindowed("Title", "", makeOptions(50), 0, 10))
	assert.Equal(t, 10, countOptionRows(out), "should render exactly maxVisible rows")
	assert.Contains(t, out, "Showing 1–10 of 50")
}

func TestRenderSelectListWindowedKeepsCursorVisible(t *testing.T) {
	t.Parallel()

	// Cursor near the end must remain within the rendered window.
	out := stripANSI(RenderSelectListWindowed("Title", "", makeOptions(50), 48, 10))
	assert.Equal(t, 10, countOptionRows(out))
	assert.Contains(t, out, "● v48", "selected option must be visible")
	assert.Contains(t, out, "Showing 41–50 of 50")
}

func TestRenderSelectListWindowedShowsAllWhenUnderCap(t *testing.T) {
	t.Parallel()

	out := stripANSI(RenderSelectListWindowed("Title", "", makeOptions(4), 0, 10))
	assert.Equal(t, 4, countOptionRows(out))
	assert.NotContains(t, out, "Showing", "no scroll indicator when everything fits")
}

func TestRenderSelectListRendersEveryOption(t *testing.T) {
	t.Parallel()

	out := stripANSI(RenderSelectList("Title", "", makeOptions(20), 0))
	assert.Equal(t, 20, countOptionRows(out))
	assert.NotContains(t, out, "Showing")
}

func TestSelectListPageAndHomeEndNavigation(t *testing.T) {
	t.Parallel()

	const maxVisible = 10
	n := maxVisible*2 + 5
	l := NewSelectList("Title", "", makeOptions(n), maxVisible)
	last := n - 1

	assert.True(t, l.HandleKey("pgdown"))
	assert.Equal(t, maxVisible, l.Cursor(), "page down jumps by a page")

	l.HandleKey("end")
	assert.Equal(t, last, l.Cursor(), "end jumps to last")

	l.HandleKey("pgdown")
	assert.Equal(t, last, l.Cursor(), "page down past end clamps")

	l.HandleKey("pgup")
	assert.Equal(t, last-maxVisible, l.Cursor(), "page up jumps back a page")

	l.HandleKey("home")
	assert.Equal(t, 0, l.Cursor(), "home jumps to top")

	l.HandleKey("pgup")
	assert.Equal(t, 0, l.Cursor(), "page up past start clamps")
}

func TestSelectListUpDownClampAndVimKeys(t *testing.T) {
	t.Parallel()

	l := NewSelectList("Title", "", makeOptions(3), 0)

	// "up" is a recognized nav key (consumed) even at the top; it just clamps.
	assert.True(t, l.HandleKey("up"))
	assert.Equal(t, 0, l.Cursor())

	assert.True(t, l.HandleKey("j"))
	assert.Equal(t, 1, l.Cursor())
	l.HandleKey("j")
	l.HandleKey("j") // past end clamps
	assert.Equal(t, 2, l.Cursor())
	l.HandleKey("k")
	assert.Equal(t, 1, l.Cursor())
}

func TestSelectListHandleKeyIgnoresNonNavKeys(t *testing.T) {
	t.Parallel()

	l := NewSelectList("Title", "", makeOptions(3), 0)
	assert.False(t, l.HandleKey("enter"))
	assert.False(t, l.HandleKey("esc"))
	assert.Equal(t, 0, l.Cursor())
}

func TestSelectListSelected(t *testing.T) {
	t.Parallel()

	l := NewSelectList("Title", "", makeOptions(3), 0)
	opt, ok := l.Selected()
	assert.True(t, ok)
	assert.Equal(t, "v0", opt.Label)

	empty := NewSelectList("Title", "", nil, 0)
	_, ok = empty.Selected()
	assert.False(t, ok)
	assert.False(t, empty.HandleKey("down"), "empty list consumes nothing")
}

func TestSelectListShortcutsIncludesPagingWhenWindowed(t *testing.T) {
	t.Parallel()

	windowed := NewSelectList("Title", "", makeOptions(50), 10).Shortcuts()
	var keys []string
	for _, s := range windowed {
		keys = append(keys, s.Key)
	}
	assert.Contains(t, keys, "PgUp/PgDn")

	short := NewSelectList("Title", "", makeOptions(3), 10).Shortcuts()
	for _, s := range short {
		assert.NotEqual(t, "PgUp/PgDn", s.Key, "no paging hint when everything fits")
	}
}

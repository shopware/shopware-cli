package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestRenderTwoColumns_ContainsBothColumns(t *testing.T) {
	body := RenderTwoColumns(80, 0, 0.38, "Left column text", "Right column text")

	assert.Contains(t, body, "Left column text")
	assert.Contains(t, body, "Right column text")
}

func TestRenderTwoColumns_HasRequestedWidth(t *testing.T) {
	body := RenderTwoColumns(80, 0, 0.38, "short", "also short")

	for _, line := range strings.Split(body, "\n") {
		assert.Equal(t, 80, lipgloss.Width(line))
	}
}

func TestRenderTwoColumns_GrowsToTallerColumn(t *testing.T) {
	body := RenderTwoColumns(80, 0, 0.38, "one\ntwo\nthree\nfour", "one line")

	assert.NotPanics(t, func() {
		RenderTwoColumns(80, 0, 0.38, "one line", "one\ntwo\nthree\nfour")
	})
	assert.Contains(t, body, "four")
	assert.Equal(t, 4, lipgloss.Height(body))
}

func TestRenderTwoColumns_StretchesToMinHeight(t *testing.T) {
	body := RenderTwoColumns(80, 10, 0.38, "one line", "one line")

	assert.Equal(t, 10, lipgloss.Height(body))
}

func TestRenderTwoColumns_MinHeightNeverTruncatesContent(t *testing.T) {
	body := RenderTwoColumns(80, 1, 0.38, "one\ntwo\nthree", "one line")

	assert.Equal(t, 3, lipgloss.Height(body))
	assert.Contains(t, body, "three")
}

func TestRenderTwoColumns_RightFractionControlsSplit(t *testing.T) {
	narrowRight := RenderTwoColumns(100, 0, 0.1, "left", "right")
	wideRight := RenderTwoColumns(100, 0, 0.8, "left", "right")

	narrowDivider := strings.IndexRune(strings.Split(narrowRight, "\n")[0], '│')
	wideDivider := strings.IndexRune(strings.Split(wideRight, "\n")[0], '│')

	assert.Greater(t, narrowDivider, wideDivider, "a smaller rightFraction should push the divider further right")
}

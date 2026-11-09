package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestRenderPhaseCardFitKeepsTheMascotWhenItFits(t *testing.T) {
	unbounded := RenderPhaseCardFit("content", 0, 0)

	assert.Equal(t, unbounded, RenderPhaseCardFit("content", PhaseCardWidth, lipgloss.Height(unbounded)))
	assert.Equal(t, unbounded, RenderPhaseCard("content"))
}

func TestRenderPhaseCardFitDropsTheMascotWhenTooShort(t *testing.T) {
	unbounded := RenderPhaseCardFit("content", 0, 0)
	maxHeight := lipgloss.Height(unbounded) - 1

	card := RenderPhaseCardFit("content", 0, maxHeight)

	assert.LessOrEqual(t, lipgloss.Height(card), maxHeight)
	assert.Contains(t, card, "content")
}

func TestRenderPhaseCardCowsayFitDropsTheMascotWhenTooShort(t *testing.T) {
	unbounded := RenderPhaseCardCowsayFit("hello", "content", 0, 0)
	maxHeight := lipgloss.Height(unbounded) - 1

	card := RenderPhaseCardCowsayFit("hello", "content", 0, maxHeight)

	assert.LessOrEqual(t, lipgloss.Height(card), maxHeight)
	assert.Contains(t, card, "content")
}

func TestRenderPhaseCardFitNarrowsToTheWidth(t *testing.T) {
	for _, maxWidth := range []int{60, minMascotCardWidth - 1} {
		card := RenderPhaseCardFit("content", maxWidth, 0)

		assert.LessOrEqual(t, lipgloss.Width(card), maxWidth)
		assert.Contains(t, card, "content")
	}
}

func TestPhaseCardContentRowsFitTheCard(t *testing.T) {
	assert.Equal(t, 0, PhaseCardContentRows(0))

	for _, maxHeight := range []int{8, 20} {
		rows := PhaseCardContentRows(maxHeight)
		content := strings.TrimSuffix(strings.Repeat("line\n", rows), "\n")

		assert.LessOrEqual(t, lipgloss.Height(RenderPhaseCardFit(content, 0, maxHeight)), maxHeight)
	}
}

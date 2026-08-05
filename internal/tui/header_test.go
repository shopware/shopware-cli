package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
)

func TestHeaderView_RightAlignsBranding(t *testing.T) {
	h := NewHeader()

	out := h.View(120)
	assert.Equal(t, 120, lipgloss.Width(out), "branding is padded to the full width")

	plain := ansi.Strip(out)
	assert.Contains(t, plain, "Shopware CLI")
	assert.True(t, strings.HasPrefix(plain, " "), "branding sits on the right, fill on the left")
}

func TestHeaderView_NarrowWidthDoesNotPanic(t *testing.T) {
	h := NewHeader()

	for _, width := range []int{0, 1, 10} {
		out := h.View(width)
		assert.Equal(t, h.width, lipgloss.Width(out), "no fill when the branding does not fit")
	}
}

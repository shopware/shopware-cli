package devtui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewCommandPalette_PopulatesFullList(t *testing.T) {
	cp := newCommandPalette()
	assert.NotNil(t, cp)
	assert.Equal(t, len(paletteCommands), len(cp.filtered))
	assert.Equal(t, 0, cp.cursor)
	assert.Empty(t, cp.filter.Value())
}

func TestCommandPalette_TypingFiltersList(t *testing.T) {
	cp := newCommandPalette()
	fullCount := len(cp.filtered)

	// Type "admin" — should narrow to entries containing "admin"
	for _, r := range "admin" {
		next, _ := cp.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		var ok bool
		cp, ok = next.(*commandPalette)
		assert.True(t, ok)
	}

	assert.Equal(t, "admin", cp.filter.Value())
	assert.Less(t, len(cp.filtered), fullCount)
	assert.NotEmpty(t, cp.filtered)
	for _, idx := range cp.filtered {
		assert.Contains(t, paletteCommands[idx].Label, "Admin")
	}
}

func TestCommandPalette_TypingNoMatchYieldsEmptyFiltered(t *testing.T) {
	cp := newCommandPalette()
	for _, r := range "zzzzz" {
		next, _ := cp.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		cp = next.(*commandPalette)
	}
	assert.Empty(t, cp.filtered)
	assert.Empty(t, cp.selectedID())
}

func TestCommandPalette_DownArrowMovesCursor(t *testing.T) {
	cp := newCommandPalette()
	assert.Equal(t, 0, cp.cursor)

	next, _ := cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	cp = next.(*commandPalette)
	assert.Equal(t, 1, cp.cursor)

	next, _ = cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	cp = next.(*commandPalette)
	assert.Equal(t, 0, cp.cursor)

	// Up at top stays at 0
	next, _ = cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
	cp = next.(*commandPalette)
	assert.Equal(t, 0, cp.cursor)
}

func TestCommandPalette_EnterEmitsSelectedID(t *testing.T) {
	cp := newCommandPalette()
	expectedID := paletteCommands[cp.filtered[0]].ID

	next, cmd := cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the palette")
	assert.NotNil(t, cmd)

	msg := cmd()
	pr, ok := msg.(paletteResultMsg)
	assert.True(t, ok, "expected paletteResultMsg, got %T", msg)
	assert.Equal(t, expectedID, pr.ID)
}

func TestCommandPalette_EnterAfterFilterEmitsFilteredID(t *testing.T) {
	cp := newCommandPalette()
	for _, r := range "clear" {
		next, _ := cp.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		cp = next.(*commandPalette)
	}
	assert.NotEmpty(t, cp.filtered)

	next, cmd := cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next)
	pr := cmd().(paletteResultMsg)
	assert.Equal(t, "cache-clear", pr.ID)
}

func TestCommandPalette_EscDismissesWithEmptyID(t *testing.T) {
	cp := newCommandPalette()
	next, cmd := cp.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.Nil(t, next, "esc should dismiss the palette")
	assert.NotNil(t, cmd)

	pr, ok := cmd().(paletteResultMsg)
	assert.True(t, ok)
	assert.Empty(t, pr.ID, "esc must not emit a command id")
}

func TestCommandPalette_ViewRendersWithoutPanic(t *testing.T) {
	cp := newCommandPalette()
	assert.NotPanics(t, func() {
		view := cp.View(120, 40)
		assert.Contains(t, view, "Commands")
	})

	// Also render after filtering to nothing
	for _, r := range "zzz" {
		next, _ := cp.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		cp = next.(*commandPalette)
	}
	assert.NotPanics(t, func() {
		view := cp.View(120, 40)
		assert.Contains(t, view, "No matching commands")
	})
}

func TestCommandPalette_NonKeyMsgIsIgnored(t *testing.T) {
	cp := newCommandPalette()
	next, cmd := cp.Update(struct{}{})
	assert.Same(t, cp, next)
	assert.Nil(t, cmd)
}

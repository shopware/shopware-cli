package devtui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewStopConfirm_DefaultsToStop(t *testing.T) {
	sc := newStopConfirm()
	assert.NotNil(t, sc)
	assert.Equal(t, stopConfirmStop, sc.selected, "default focus should be on 'Stop containers & quit'")
}

func TestStopConfirm_RightArrowAdvancesSelection(t *testing.T) {
	sc := newStopConfirm()
	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	assert.Same(t, sc, next)
	assert.Nil(t, cmd)
	assert.Equal(t, stopConfirmQuit, sc.selected)

	next, _ = sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmCancel, sc.selected)
}

func TestStopConfirm_RightArrowClampsAtLastOption(t *testing.T) {
	sc := newStopConfirm()
	sc.selected = stopConfirmCancel
	next, _ := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmCancel, sc.selected, "right arrow must not move past the last option")
}

func TestStopConfirm_LeftArrowMovesBackAndClamps(t *testing.T) {
	sc := newStopConfirm()
	sc.selected = stopConfirmCancel
	next, _ := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmQuit, sc.selected)

	next, _ = sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmStop, sc.selected)

	next, _ = sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmStop, sc.selected, "left arrow must not move before the first option")
}

func TestStopConfirm_TabCyclesSelection(t *testing.T) {
	sc := newStopConfirm()
	assert.Equal(t, stopConfirmStop, sc.selected)

	next, _ := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmQuit, sc.selected)

	next, _ = sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmCancel, sc.selected)

	next, _ = sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmStop, sc.selected, "tab should wrap back to the first option")
}

func TestStopConfirm_EnterOnStopEmitsStopTrue(t *testing.T) {
	sc := newStopConfirm()
	assert.Equal(t, stopConfirmStop, sc.selected)

	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the modal")
	assert.NotNil(t, cmd)

	res, ok := cmd().(stopConfirmResultMsg)
	assert.True(t, ok, "expected stopConfirmResultMsg, got %T", cmd())
	assert.True(t, res.Stop)
	assert.False(t, res.Cancel)
}

func TestStopConfirm_EnterOnQuitEmitsStopFalse(t *testing.T) {
	sc := newStopConfirm()
	sc.selected = stopConfirmQuit

	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss even when the user quits")
	assert.NotNil(t, cmd)

	res, ok := cmd().(stopConfirmResultMsg)
	assert.True(t, ok)
	assert.False(t, res.Stop)
	assert.False(t, res.Cancel)
}

func TestStopConfirm_EnterOnCancelEmitsCancel(t *testing.T) {
	sc := newStopConfirm()
	sc.selected = stopConfirmCancel

	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the modal")
	assert.NotNil(t, cmd)

	res, ok := cmd().(stopConfirmResultMsg)
	assert.True(t, ok)
	assert.True(t, res.Cancel, "selecting Cancel should emit a cancel result")
	assert.False(t, res.Stop)
}

func TestStopConfirm_HAndLKeysMoveSelection(t *testing.T) {
	sc := newStopConfirm()
	sc.selected = stopConfirmStop

	next, _ := sc.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmQuit, sc.selected, "'l' should advance the selection")

	next, _ = sc.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	sc = next.(*stopConfirm)
	assert.Equal(t, stopConfirmStop, sc.selected, "'h' should move the selection back")
}

func TestStopConfirm_EscEmitsCancel(t *testing.T) {
	sc := newStopConfirm()

	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.Nil(t, next, "esc should dismiss the modal")
	assert.NotNil(t, cmd)

	res, ok := cmd().(stopConfirmResultMsg)
	assert.True(t, ok, "expected stopConfirmResultMsg, got %T", cmd())
	assert.True(t, res.Cancel, "esc should emit a cancel result")
	assert.False(t, res.Stop)
}

func TestStopConfirm_NonKeyMsgIsIgnored(t *testing.T) {
	sc := newStopConfirm()
	next, cmd := sc.Update(struct{}{})
	assert.Same(t, sc, next)
	assert.Nil(t, cmd)
}

func TestStopConfirm_ViewRendersWithoutPanic(t *testing.T) {
	sc := newStopConfirm()
	assert.NotPanics(t, func() {
		view := sc.View(120, 40)
		assert.Contains(t, view, "Leaving the workspace")
		assert.Contains(t, view, "Stop containers & quit")
		assert.Contains(t, view, "Quit, keep running")
		assert.Contains(t, view, "Cancel")
	})

	sc.selected = stopConfirmCancel
	assert.NotPanics(t, func() {
		_ = sc.View(120, 40)
	})
}

package devtui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestNewStopConfirm_DefaultsToYes(t *testing.T) {
	sc := newStopConfirm()
	assert.NotNil(t, sc)
	assert.True(t, sc.yes, "default focus should be on 'Stop containers & quit'")
}

func TestStopConfirm_RightArrowSelectsNo(t *testing.T) {
	sc := newStopConfirm()
	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	assert.Same(t, sc, next)
	assert.Nil(t, cmd)
	assert.False(t, sc.yes)
}

func TestStopConfirm_LeftArrowSelectsYes(t *testing.T) {
	sc := newStopConfirm()
	sc.yes = false
	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
	assert.Same(t, sc, next)
	assert.Nil(t, cmd)
	assert.True(t, sc.yes)
}

func TestStopConfirm_TabTogglesSelection(t *testing.T) {
	sc := newStopConfirm()
	assert.True(t, sc.yes)

	next, _ := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	sc = next.(*stopConfirm)
	assert.False(t, sc.yes)

	next, _ = sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	sc = next.(*stopConfirm)
	assert.True(t, sc.yes)
}

func TestStopConfirm_EnterOnConfirmEmitsStopTrue(t *testing.T) {
	sc := newStopConfirm()
	assert.True(t, sc.yes)

	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the modal")
	assert.NotNil(t, cmd)

	res, ok := cmd().(stopConfirmResultMsg)
	assert.True(t, ok, "expected stopConfirmResultMsg, got %T", cmd())
	assert.True(t, res.Stop)
}

func TestStopConfirm_EnterOnCancelEmitsStopFalse(t *testing.T) {
	sc := newStopConfirm()
	// Move focus to "Quit, keep running"
	next, _ := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	sc = next.(*stopConfirm)
	assert.False(t, sc.yes)

	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss even when the user cancels")
	assert.NotNil(t, cmd)

	res, ok := cmd().(stopConfirmResultMsg)
	assert.True(t, ok)
	assert.False(t, res.Stop)
}

func TestStopConfirm_HKeySelectsYesAndLKeySelectsNo(t *testing.T) {
	sc := newStopConfirm()
	sc.yes = false

	next, _ := sc.Update(tea.KeyPressMsg(tea.Key{Code: 'h', Text: "h"}))
	sc = next.(*stopConfirm)
	assert.True(t, sc.yes, "'h' should select Yes")

	next, _ = sc.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Text: "l"}))
	sc = next.(*stopConfirm)
	assert.False(t, sc.yes, "'l' should select No")
}

func TestStopConfirm_EscIsNoop(t *testing.T) {
	// NOTE: stopConfirm.Update does not handle Esc — pressing it leaves the
	// modal in place and emits no command. The model-level update layer is
	// responsible for any global Esc handling.
	sc := newStopConfirm()
	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.Same(t, sc, next)
	assert.Nil(t, cmd)
	assert.True(t, sc.yes, "esc must not mutate the selection")
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
	})

	sc.yes = false
	assert.NotPanics(t, func() {
		_ = sc.View(120, 40)
	})
}

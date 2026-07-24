package devtui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/tui/prompt"
)

// Button mechanics (arrows, tab, clamping) are covered by the prompt package
// tests; here only devtui's stop-confirm contract is pinned: the choice IDs
// the result handler dispatches on, and the default focus.

func TestStopConfirm_DefaultConfirmsStop(t *testing.T) {
	sc := newStopConfirm()

	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	assert.Nil(t, next, "enter should dismiss the modal")
	require.NotNil(t, cmd)

	res, ok := cmd().(prompt.ResultMsg)
	require.True(t, ok, "expected prompt.ResultMsg, got %T", cmd())
	assert.Equal(t, stopConfirmID, res.ID)
	assert.Equal(t, stopConfirmStop, res.Choice, "default focus is 'Stop containers & quit'")
}

func TestStopConfirm_EscDismissesWithoutChoice(t *testing.T) {
	sc := newStopConfirm()

	next, cmd := sc.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEsc}))
	assert.Nil(t, next)
	require.NotNil(t, cmd)

	res := cmd().(prompt.ResultMsg)
	assert.Empty(t, res.Choice)
}

func TestStopConfirm_ViewRendersAllChoices(t *testing.T) {
	sc := newStopConfirm()

	view := ansi.Strip(sc.View(120, 40))
	assert.Contains(t, view, "Leaving the workspace")
	assert.Contains(t, view, "Stop containers & quit")
	assert.Contains(t, view, "Quit, keep running")
	assert.Contains(t, view, "Cancel")
}

func TestHandleStopConfirmResult_Choices(t *testing.T) {
	// Cancel (or dismissal) keeps the dashboard running.
	m := newTestModel()
	updated, cmd := m.handleStopConfirmResult(prompt.ResultMsg{ID: stopConfirmID, Choice: stopConfirmCancel})
	assert.Equal(t, phaseDashboard, updated.(Model).phase)
	assert.Nil(t, cmd)

	// Quit keeps containers running and exits.
	m = newTestModel()
	_, cmd = m.handleStopConfirmResult(prompt.ResultMsg{ID: stopConfirmID, Choice: stopConfirmQuit})
	require.NotNil(t, cmd)
	_, isQuit := cmd().(tea.QuitMsg)
	assert.True(t, isQuit)

	// Stop enters the stopping phase instead of quitting outright.
	m = newTestModel()
	updated, cmd = m.handleStopConfirmResult(prompt.ResultMsg{ID: stopConfirmID, Choice: stopConfirmStop})
	assert.Equal(t, phaseStopping, updated.(Model).phase)
	assert.NotNil(t, cmd)

	// Results from other prompts are ignored.
	m = newTestModel()
	updated, cmd = m.handleStopConfirmResult(prompt.ResultMsg{ID: "other", Choice: stopConfirmStop})
	assert.Equal(t, phaseDashboard, updated.(Model).phase)
	assert.Nil(t, cmd)
}

package sqlshell

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestInteractiveModel(t *testing.T) *interactiveModel {
	t.Helper()

	return newInteractiveModel(t.Context(), openFakeDB(t), FormatTSV)
}

func typeText(m *interactiveModel, text string) {
	for _, r := range text {
		m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func pressKey(m *interactiveModel, key tea.Key) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg(key))
	return cmd
}

func TestInteractiveDeleteWordBackward(t *testing.T) {
	m := newTestInteractiveModel(t)

	typeText(m, "SELECT id FROM product")

	pressKey(m, tea.Key{Code: tea.KeyBackspace, Mod: tea.ModAlt})
	assert.Equal(t, "SELECT id FROM ", m.input.Value())

	pressKey(m, tea.Key{Code: 'w', Mod: tea.ModCtrl})
	assert.Equal(t, "SELECT id ", m.input.Value())
}

func TestInteractiveDeleteWordForward(t *testing.T) {
	m := newTestInteractiveModel(t)

	typeText(m, "SELECT id FROM product")
	m.input.CursorStart()

	pressKey(m, tea.Key{Code: 'd', Mod: tea.ModAlt})
	assert.Equal(t, " id FROM product", m.input.Value())
}

func TestInteractiveHistoryNavigation(t *testing.T) {
	m := newTestInteractiveModel(t)

	typeText(m, "SELECT 1;")
	pressKey(m, tea.Key{Code: tea.KeyEnter})
	m.Update(resultMsg{output: ""})

	typeText(m, "SEL")
	pressKey(m, tea.Key{Code: tea.KeyUp})
	assert.Equal(t, "SELECT 1;", m.input.Value(), "up recalls the previous line")

	pressKey(m, tea.Key{Code: tea.KeyDown})
	assert.Equal(t, "SEL", m.input.Value(), "down restores the stashed live input")
}

func TestInteractiveContinuationAndExecution(t *testing.T) {
	m := newTestInteractiveModel(t)

	typeText(m, "SELECT 1")
	pressKey(m, tea.Key{Code: tea.KeyEnter})

	assert.False(t, m.running, "incomplete statement must not execute")
	assert.Equal(t, promptContinuation, m.prompt())

	typeText(m, ";")
	pressKey(m, tea.Key{Code: tea.KeyEnter})

	assert.True(t, m.running)
	assert.Empty(t, m.buffer)
}

func TestInteractiveExitCommand(t *testing.T) {
	m := newTestInteractiveModel(t)

	typeText(m, "exit")
	pressKey(m, tea.Key{Code: tea.KeyEnter})

	assert.True(t, m.done)
}

func TestInteractiveCtrlCClearsPendingInput(t *testing.T) {
	m := newTestInteractiveModel(t)

	typeText(m, "SELECT 1")
	pressKey(m, tea.Key{Code: tea.KeyEnter})
	typeText(m, "FROM t")

	pressKey(m, tea.Key{Code: 'c', Mod: tea.ModCtrl})

	assert.False(t, m.done)
	assert.Empty(t, m.buffer)
	assert.Empty(t, m.input.Value())
	assert.Equal(t, promptMain, m.prompt())
}

func TestInteractiveCtrlCQuitsWhenIdle(t *testing.T) {
	m := newTestInteractiveModel(t)

	pressKey(m, tea.Key{Code: 'c', Mod: tea.ModCtrl})

	assert.True(t, m.done)
}

func TestInteractiveCtrlDRunsTrailingStatement(t *testing.T) {
	m := newTestInteractiveModel(t)

	typeText(m, "UPDATE t SET one_row = 1")
	pressKey(m, tea.Key{Code: tea.KeyEnter})

	pressKey(m, tea.Key{Code: 'd', Mod: tea.ModCtrl})

	assert.True(t, m.running, "pending statement must run on EOF")
	assert.Empty(t, m.buffer)
}

func TestInteractiveExecuteRendersResults(t *testing.T) {
	fakeQueries.Store("SELECT 3", fakeTable{
		cols:  []string{"3"},
		types: []string{"BIGINT"},
		rows:  [][]driver.Value{{[]byte("3")}},
	})

	m := newTestInteractiveModel(t)

	cmd := m.startExecution([]string{"SELECT 3", "SELECT boom"}, true)
	require.True(t, m.running)

	result, ok := cmd().(resultMsg)
	require.True(t, ok)

	assert.True(t, result.quit)
	assert.Contains(t, result.output, "3\n3")
	assert.Contains(t, result.output, "ERROR: query exploded")
}

func TestInteractiveCtrlCCancelsRunningQuery(t *testing.T) {
	m := newTestInteractiveModel(t)

	cmd := m.startExecution([]string{"SELECT block"}, false)
	require.True(t, m.running)

	// The statement blocks until its context is cancelled.
	results := make(chan tea.Msg, 1)
	go func() { results <- cmd() }()

	pressKey(m, tea.Key{Code: 'c', Mod: tea.ModCtrl})
	assert.True(t, m.cancelling)

	result, ok := (<-results).(resultMsg)
	require.True(t, ok)
	assert.Contains(t, result.output, "Query cancelled")

	m.Update(result)
	assert.False(t, m.running)
	assert.False(t, m.cancelling)
	assert.Nil(t, m.cancelRun)
}

func TestFilterInteractiveErr(t *testing.T) {
	assert.NoError(t, filterInteractiveErr(nil))
	assert.NoError(t, filterInteractiveErr(fmt.Errorf("%w: %w", tea.ErrProgramKilled, context.Canceled)))

	other := errors.New("terminal broke")
	assert.Equal(t, other, filterInteractiveErr(other))
}

func TestInteractiveIgnoresKeysWhileRunning(t *testing.T) {
	m := newTestInteractiveModel(t)
	m.running = true

	typeText(m, "SELECT 1")

	assert.Empty(t, m.input.Value())
}

func TestInteractiveResultQuits(t *testing.T) {
	m := newTestInteractiveModel(t)
	m.running = true

	_, cmd := m.Update(resultMsg{output: "done", quit: true})

	assert.False(t, m.running)
	assert.True(t, m.done)
	require.NotNil(t, cmd)
}

func TestInteractiveViewShowsRunningState(t *testing.T) {
	m := newTestInteractiveModel(t)

	assert.Contains(t, m.View().Content, promptMain)

	m.running = true
	assert.Contains(t, m.View().Content, "Executing")
	assert.Contains(t, m.View().Content, "ctrl+c")

	m.cancelling = true
	assert.Contains(t, m.View().Content, "Cancelling")

	m.done = true
	assert.Equal(t, "", strings.TrimSpace(m.View().Content))
}

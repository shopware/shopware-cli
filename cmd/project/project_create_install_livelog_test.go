package project

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func newTestInstallModel() installLogModel {
	m := newInstallLogModel("Installing dependencies", nil)
	m.width = 80
	return m
}

func applyInstall(t *testing.T, m installLogModel, msg tea.Msg) (installLogModel, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	nm, ok := next.(installLogModel)
	assert.True(t, ok, "Update should return installLogModel, got %T", next)
	return nm, cmd
}

func keyPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)})
}

func ctrlKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: r, Mod: tea.ModCtrl})
}

func TestInstallLogModel_ToggleLogs(t *testing.T) {
	t.Parallel()
	m := newTestInstallModel()
	assert.False(t, m.showLog)

	m, _ = applyInstall(t, m, keyPress('l'))
	assert.True(t, m.showLog, "l should reveal the live log")

	m, _ = applyInstall(t, m, keyPress('l'))
	assert.False(t, m.showLog, "l should hide the live log again")

	m, _ = applyInstall(t, m, ctrlKey('l'))
	assert.True(t, m.showLog, "ctrl+l should also toggle the live log")
}

func TestInstallLogModel_CtrlCInterrupts(t *testing.T) {
	t.Parallel()
	m := newTestInstallModel()

	_, cmd := applyInstall(t, m, ctrlKey('c'))
	assert.NotNil(t, cmd)
	_, ok := cmd().(tea.InterruptMsg)
	assert.True(t, ok, "ctrl+c should interrupt the program")
}

func TestInstallLogModel_LineAppendKeepsReading(t *testing.T) {
	t.Parallel()
	m := newTestInstallModel()

	m, cmd := applyInstall(t, m, installLineMsg("hello"))
	assert.Equal(t, []string{"hello"}, m.lines)
	assert.NotNil(t, cmd, "a line should schedule the next read")

	m, _ = applyInstall(t, m, installLineMsg("world"))
	assert.Equal(t, []string{"hello", "world"}, m.lines)
}

func TestInstallLogModel_DoneQuitsAndCollapses(t *testing.T) {
	t.Parallel()
	m := newTestInstallModel()
	wantErr := errors.New("boom")

	m, cmd := applyInstall(t, m, installDoneMsg{err: wantErr, output: []string{"a", "b"}})
	assert.True(t, m.done)
	assert.Same(t, wantErr, m.runErr)
	assert.Equal(t, []string{"a", "b"}, m.output)

	assert.NotNil(t, cmd)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok, "done should quit the program")

	// Once finished the inline area collapses so the surrounding output is clean.
	assert.Equal(t, "", m.View().Content)
}

func TestInstallLogModel_ViewDefaultHidesLog(t *testing.T) {
	t.Parallel()
	m := newTestInstallModel()
	m.lines = []string{"some output"}

	content := m.View().Content
	assert.Contains(t, content, "Installing dependencies")
	assert.Contains(t, content, "press l to show live log")
	assert.NotContains(t, content, "Live log", "the panel must stay hidden until toggled")
	assert.NotContains(t, content, "some output")
}

func TestInstallLogModel_ViewWithLogsShowsPanel(t *testing.T) {
	t.Parallel()
	m := newTestInstallModel()
	m.showLog = true
	m.lines = []string{"first line", "second line"}

	content := m.View().Content
	assert.Contains(t, content, "press l to hide live log")
	assert.Contains(t, content, "Live log")
	assert.Contains(t, content, "second line")
}

func TestInstallLogModel_LogPanelTailsRecentLines(t *testing.T) {
	t.Parallel()
	m := newTestInstallModel()
	m.showLog = true

	var lines []string
	for i := 0; i < installLogVisibleLines+3; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	m.lines = lines

	panel := m.renderLogPanel()
	assert.NotContains(t, panel, "line-00", "oldest lines should be dropped from the tail")
	assert.Contains(t, panel, fmt.Sprintf("line-%02d", installLogVisibleLines+2), "the newest line must be shown")
}

func TestInstallLogModel_LogPanelTruncatesLongLines(t *testing.T) {
	t.Parallel()
	m := newTestInstallModel()
	m.width = 24
	m.showLog = true
	longLine := strings.Repeat("x", 100)
	m.lines = []string{longLine}

	panel := m.renderLogPanel()
	assert.NotContains(t, panel, longLine, "long lines must not be rendered in full")
	assert.Contains(t, panel, "…", "truncated lines should show an ellipsis")
}

func TestStreamCombinedOutput_StreamsAndReturnsOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on a POSIX shell")
	}
	t.Parallel()

	ch := make(chan string, 64)
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "echo hello; echo world")

	lines, err := streamCombinedOutput(cmd, ch)
	assert.NoError(t, err)
	assert.Equal(t, []string{"hello", "world"}, lines)

	var streamed []string
	for l := range ch {
		streamed = append(streamed, l)
	}
	assert.Equal(t, []string{"hello", "world"}, streamed, "every line should also be streamed to the channel")
}

func TestStreamCombinedOutput_FailureCapturesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("relies on a POSIX shell")
	}
	t.Parallel()

	ch := make(chan string, 64)
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "echo oops >&2; exit 3")

	lines, err := streamCombinedOutput(cmd, ch)
	assert.Error(t, err, "a non-zero exit must be reported")
	assert.Contains(t, lines, "oops", "merged stderr should be captured for error reporting")
}

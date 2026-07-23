package tui

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteFailureOutput(t *testing.T) {
	var output bytes.Buffer

	writeFailureOutput(
		&output,
		"Installing dependencies",
		errors.New("exit status 1"),
		[]string{"Loading composer repositories", "Your requirements could not be resolved."},
	)

	assert.Equal(t, "\n✗ Installing dependencies\n\n  Command failed: exit status 1\n  Loading composer repositories\n  Your requirements could not be resolved.\n", output.String())
}

func TestWriteFailureOutputWithoutCommandLogs(t *testing.T) {
	var output bytes.Buffer

	writeFailureOutput(&output, "Installing dependencies", errors.New("signal: killed"), nil)

	assert.Equal(t, "\n✗ Installing dependencies\n\n  Command failed: signal: killed\n", output.String())
}

func TestRunSpinnerWithLogsWithCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := RunSpinnerWithLogs(ctx, "Installing dependencies", exec.CommandContext(ctx, "false"))

	assert.ErrorContains(t, err, "error opening TTY")
}

func TestRunSpinnerWithLogs(t *testing.T) {
	t.Run("returns program errors", func(t *testing.T) {
		wantErr := errors.New("renderer failed")
		err := runSpinnerWithLogs(t.Context(), "Installing dependencies", exec.CommandContext(t.Context(), "false"), &bytes.Buffer{}, func(context.Context, tea.Model) error {
			return wantErr
		})
		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns cancellation", func(t *testing.T) {
		err := runSpinnerWithLogs(t.Context(), "Installing dependencies", exec.CommandContext(t.Context(), "false"), &bytes.Buffer{}, func(_ context.Context, model tea.Model) error {
			model.(*installProgressModel).cancelled = true
			return nil
		})
		assert.ErrorIs(t, err, context.Canceled)
	})

	t.Run("prints command failures", func(t *testing.T) {
		var output bytes.Buffer
		wantErr := errors.New("exit status 1")
		err := runSpinnerWithLogs(t.Context(), "Installing dependencies", exec.CommandContext(t.Context(), "false"), &output, func(_ context.Context, model tea.Model) error {
			progress := model.(*installProgressModel)
			_, err := progress.logWriter.Write([]byte("composer error\n"))
			require.NoError(t, err)
			progress.err = wantErr
			return nil
		})
		assert.ErrorIs(t, err, wantErr)
		assert.Contains(t, output.String(), "composer error")
	})

	t.Run("returns success", func(t *testing.T) {
		err := runSpinnerWithLogs(t.Context(), "Installing dependencies", exec.CommandContext(t.Context(), "false"), &bytes.Buffer{}, func(context.Context, tea.Model) error {
			return nil
		})
		assert.NoError(t, err)
	})
}

func TestRunSpinnerWithLogsTeesPresetWriters(t *testing.T) {
	var stdout, stderr bytes.Buffer

	cmd := exec.CommandContext(t.Context(), "sh", "-c", "echo out; echo err >&2")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	var logLines []string
	err := runSpinnerWithLogs(t.Context(), "Installing dependencies", cmd, &bytes.Buffer{}, func(_ context.Context, model tea.Model) error {
		progress := model.(*installProgressModel)
		progress.err = progress.cmd.Run()
		logLines = progress.logWriter.GetLastLines(10)
		return nil
	})
	require.NoError(t, err)

	// The caller's writers still receive the command output...
	assert.Equal(t, "out\n", stdout.String())
	assert.Equal(t, "err\n", stderr.String())

	// ...and the spinner's log writer captures it for the live log view.
	assert.ElementsMatch(t, []string{"out", "err"}, logLines)
}

func TestLogWriterStoresAndTrimsLines(t *testing.T) {
	var writer logWriter

	n, err := writer.Write([]byte("first\r\nsecond"))
	require.NoError(t, err)
	assert.Equal(t, len("first\r\nsecond"), n)
	assert.Equal(t, []string{"first", "second"}, writer.GetLastLines(10))

	for i := range 101 {
		_, err = writer.Write([]byte(strings.Repeat("x", i%3+1) + "\n"))
		require.NoError(t, err)
	}

	lines := writer.GetLastLines(100)
	assert.Len(t, lines, 100)
	assert.Equal(t, "xx", lines[0])
	assert.Equal(t, "xx", lines[len(lines)-1])
	assert.Len(t, writer.GetLastLines(101), 100)
	assert.Equal(t, []string{"xx"}, writer.GetLastLines(1))
}

func TestInstallProgressModelInitRunsCommand(t *testing.T) {
	model := newProgressModel(exec.CommandContext(t.Context(), "false"))

	batch, ok := model.Init()().(tea.BatchMsg)
	require.True(t, ok)
	require.Len(t, batch, 2)
	_, ok = batch[0]().(spinner.TickMsg)
	assert.True(t, ok)

	finished, ok := batch[1]().(installFinishedMsg)
	require.True(t, ok)
	assert.Error(t, finished.err)
}

func TestInstallProgressModelUpdate(t *testing.T) {
	model := newProgressModel(exec.CommandContext(t.Context(), "false"))

	updated, cmd := model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	assert.Same(t, model, updated)
	assert.Nil(t, cmd)
	assert.Equal(t, 80, model.width)
	assert.Equal(t, 24, model.height)

	_, cmd = model.Update(tea.KeyPressMsg(tea.Key{Code: 'l', Mod: tea.ModCtrl}))
	assert.Nil(t, cmd)
	assert.True(t, model.showLogs)

	_, cmd = model.Update(tea.KeyPressMsg(tea.Key{Code: 'x'}))
	assert.Nil(t, cmd)

	tick := model.spinner.Tick()
	_, cmd = model.Update(tick)
	assert.NotNil(t, cmd)

	wantErr := errors.New("install failed")
	_, cmd = model.Update(installFinishedMsg{err: wantErr})
	assert.True(t, model.done)
	assert.ErrorIs(t, model.err, wantErr)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestInstallProgressModelCancels(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	model := newProgressModel(exec.CommandContext(t.Context(), "false"))
	model.cancel = cancel

	_, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
	assert.True(t, model.cancelled)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
	_, ok := cmd().(tea.QuitMsg)
	assert.True(t, ok)
}

func TestInstallProgressModelView(t *testing.T) {
	t.Run("successful completion", func(t *testing.T) {
		model := newProgressModel(nil)
		model.done = true
		view := model.View().Content
		assert.Contains(t, view, "✔")
		assert.Contains(t, view, "Installing dependencies")
	})

	t.Run("failed completion", func(t *testing.T) {
		model := newProgressModel(nil)
		_, err := model.logWriter.Write([]byte("composer error\n"))
		require.NoError(t, err)
		model.done = true
		model.err = errors.New("exit status 1")
		view := model.View().Content
		assert.Contains(t, view, "✗")
		assert.Contains(t, view, "Installing dependencies")
		assert.Contains(t, view, "composer error")
	})

	t.Run("running without logs", func(t *testing.T) {
		model := newProgressModel(nil)
		assert.Contains(t, model.View().Content, "Ctrl+L to see live log")
	})

	t.Run("running with empty logs", func(t *testing.T) {
		model := newProgressModel(nil)
		model.showLogs = true
		assert.Contains(t, model.View().Content, "Waiting for output...")
	})

	t.Run("running with logs in a wide terminal", func(t *testing.T) {
		model := newProgressModel(nil)
		_, err := model.logWriter.Write([]byte("first\nsecond\n"))
		require.NoError(t, err)
		model.showLogs = true
		model.width = 200
		view := model.View().Content
		assert.Contains(t, view, "Ctrl+L to hide live log")
		assert.Contains(t, view, "first")
		assert.Contains(t, view, "second")
	})
}

func newProgressModel(cmd *exec.Cmd) *installProgressModel {
	return &installProgressModel{
		spinner:   spinner.New(),
		logWriter: &logWriter{},
		title:     "Installing dependencies",
		cmd:       cmd,
		cancel:    func() {},
	}
}

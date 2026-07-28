package tui

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskStart_ResetsState(t *testing.T) {
	task := NewTask("Building...")
	task.lines = []string{"old"}
	task.done = true
	task.err = errors.New("stale")

	cmd := task.Start(func() (*exec.Cmd, error) {
		return nil, errors.New("factory failed")
	})

	assert.Equal(t, "Building...", task.Title)
	assert.False(t, task.Done())
	assert.Nil(t, task.Err())
	assert.Empty(t, task.Lines())
	assert.NotNil(t, cmd)
}

func TestTaskStart_FactoryErrorEmitsDoneMsg(t *testing.T) {
	task := NewTask("Test")

	wantErr := errors.New("factory failed")
	cmd := task.Start(func() (*exec.Cmd, error) {
		return nil, wantErr
	})
	require.NotNil(t, cmd)

	// tea.Batch returns a BatchMsg containing the individual cmds when invoked:
	// reader + runner + spinner tick.
	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok, "Start should return a tea.Batch")
	require.Len(t, batch, 3)

	// The reader and runner must run concurrently: the runner closes the
	// channel, which unblocks the reader.
	results := make(chan tea.Msg, 2)
	for _, c := range batch[:2] {
		go func() { results <- c() }()
	}

	got := make(map[string]tea.Msg)
	for range 2 {
		select {
		case msg := <-results:
			switch v := msg.(type) {
			case TaskDoneMsg:
				got["done"] = v
			case taskStreamClosedMsg:
				got["stream-closed"] = v
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for batched cmds")
		}
	}

	done, ok := got["done"].(TaskDoneMsg)
	require.True(t, ok, "expected TaskDoneMsg from one of the batched cmds")
	assert.Same(t, wantErr, done.Err)
	_, hasClosed := got["stream-closed"]
	assert.True(t, hasClosed, "expected the stream-closed msg from the reader cmd")

	// The spinner tick is the last batched cmd so long-running commands with
	// no early output never look frozen.
	_, isTick := batch[2]().(spinner.TickMsg)
	assert.True(t, isTick, "last batched cmd should be the spinner tick")
}

func TestTask_StreamsLinesAndCompletes(t *testing.T) {
	task := NewTask("Echo")

	cmd := task.Start(func() (*exec.Cmd, error) {
		return exec.CommandContext(context.Background(), "echo", "hello"), nil
	})

	batch, ok := cmd().(tea.BatchMsg)
	require.True(t, ok)
	require.Len(t, batch, 3)

	// Run the runner concurrently, then pump the reader through Update until
	// the stream closes — exactly what the Bubble Tea loop would do.
	runnerDone := make(chan tea.Msg, 1)
	go func() { runnerDone <- batch[1]() }()

	read := batch[0]
	deadline := time.Now().Add(5 * time.Second)
	for read != nil && time.Now().Before(deadline) {
		msg := read()
		if _, closed := msg.(taskStreamClosedMsg); closed {
			break
		}
		task, read = task.Update(msg)
	}

	select {
	case msg := <-runnerDone:
		task, _ = task.Update(msg)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the runner")
	}

	assert.True(t, task.Done())
	assert.NoError(t, task.Err())
	assert.Contains(t, task.Lines(), "hello")
}

func TestTask_UpdateLinesAreCapped(t *testing.T) {
	task := NewTask("t")
	for range taskLogKeep + 10 {
		task, _ = task.Update(TaskLineMsg{Line: "x"})
	}
	assert.Len(t, task.Lines(), taskLogKeep)
}

func TestTask_StatusTitleAndSpinnerStopWhenDone(t *testing.T) {
	task := NewTask("Building...")
	assert.NotEqual(t, "Building...", task.StatusTitle(), "running tasks carry the spinner prefix")

	task, _ = task.Update(TaskDoneMsg{})
	assert.Equal(t, "Building...", task.StatusTitle())

	_, cmd := task.Update(spinner.TickMsg{})
	assert.Nil(t, cmd, "ticks stop once the task is done so the output stays static")
}

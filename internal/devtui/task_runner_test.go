package devtui

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestTaskDoneMsg_PlainStruct(t *testing.T) {
	msg := taskDoneMsg{}
	assert.Nil(t, msg.err)

	want := errors.New("boom")
	withErr := taskDoneMsg{err: want}
	assert.Same(t, want, withErr.err)
}

func TestRunTask_SetsPhaseAndResetsState(t *testing.T) {
	m := &Model{
		phase:        phaseDashboard,
		taskDone:     true,
		taskErr:      errors.New("stale"),
		overlayLines: []string{"old", "lines"},
	}

	// Provide a task that returns an error from the factory itself so the
	// goroutine path is exercised but doesn't actually launch a process.
	cmd := m.runTask("Building...", func() (*exec.Cmd, error) {
		return nil, errors.New("factory failed")
	})

	assert.Equal(t, phaseTask, m.phase)
	assert.Equal(t, "Building...", m.taskTitle)
	assert.False(t, m.taskDone)
	assert.Nil(t, m.taskErr)
	assert.Empty(t, m.overlayLines)
	assert.NotNil(t, m.dockerOutChan)
	assert.NotNil(t, cmd)
}

func TestRunTask_FactoryErrorEmitsTaskDoneMsg(t *testing.T) {
	m := &Model{}

	wantErr := errors.New("factory failed")
	cmd := m.runTask("Test", func() (*exec.Cmd, error) {
		return nil, wantErr
	})
	assert.NotNil(t, cmd)

	// tea.Batch returns a BatchMsg containing the individual cmds when invoked.
	batchMsg := cmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	assert.True(t, ok, "runTask should return a tea.Batch, got %T", batchMsg)
	// reader + runner + spinner tick; the tick is last and is a no-op here.
	assert.Len(t, batch, 3)

	// The first two batched commands must run concurrently: the doneCmd closes
	// the channel, which unblocks readFromChan.
	batch = batch[:2]
	type result struct{ msg tea.Msg }
	results := make(chan result, 2)
	for _, c := range batch {
		c := c
		go func() { results <- result{msg: c()} }()
	}

	got := make(map[string]tea.Msg)
	for i := 0; i < 2; i++ {
		select {
		case r := <-results:
			switch v := r.msg.(type) {
			case taskDoneMsg:
				got["done"] = v
			case dockerOutputDoneMsg:
				got["chan-done"] = v
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for batched cmds")
		}
	}

	done, ok := got["done"].(taskDoneMsg)
	assert.True(t, ok, "expected taskDoneMsg from one of the batched cmds")
	assert.Same(t, wantErr, done.err)
	_, hasChan := got["chan-done"]
	assert.True(t, hasChan, "expected dockerOutputDoneMsg from the channel-reader cmd")
}

func TestRunTask_SmokeEchoStreamsLinesAndCompletes(t *testing.T) {
	// Smoke test using `echo` to verify the streaming path end-to-end.
	// Cross-platform: `echo` exists on both Unix and Windows (cmd builtin),
	// but we restrict to a path that works on darwin/linux runners.
	m := &Model{}

	cmd := m.runTask("Echo", func() (*exec.Cmd, error) {
		return exec.CommandContext(context.Background(), "echo", "hello"), nil
	})

	batchMsg := cmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	assert.True(t, ok)
	// reader + runner + spinner tick; only the first two are exercised below.
	assert.Len(t, batch, 3)

	// Identify which cmd is the channel reader vs which is the runner by
	// invoking both concurrently and accumulating messages.
	type result struct {
		idx int
		msg tea.Msg
	}
	results := make(chan result, 4)

	// Kick off the runner (doneCmd) — it must run before we can drain.
	go func() {
		// readFromChan was the first arg to tea.Batch; run it repeatedly
		// until the channel closes.
		for {
			msg := batch[0]()
			results <- result{idx: 0, msg: msg}
			if _, done := msg.(dockerOutputDoneMsg); done {
				return
			}
		}
	}()
	go func() {
		results <- result{idx: 1, msg: batch[1]()}
	}()

	var lines []string
	var done taskDoneMsg
	gotDone := false
	gotChanDone := false
	deadline := time.After(5 * time.Second)
loop:
	for {
		select {
		case r := <-results:
			switch v := r.msg.(type) {
			case dockerOutputLineMsg:
				lines = append(lines, string(v))
			case dockerOutputDoneMsg:
				gotChanDone = true
			case taskDoneMsg:
				done = v
				gotDone = true
			}
			if gotDone && gotChanDone {
				break loop
			}
		case <-deadline:
			t.Fatalf("timed out waiting for echo task to finish (lines=%v, done=%v, chanDone=%v)", lines, gotDone, gotChanDone)
		}
	}

	assert.NoError(t, done.err)
	assert.Contains(t, lines, "hello")
}

func TestRunTask_StartsSpinnerTick(t *testing.T) {
	m := &Model{}
	cmd := m.runTask("Building...", func() (*exec.Cmd, error) {
		return nil, errors.New("factory failed")
	})

	batch, ok := cmd().(tea.BatchMsg)
	assert.True(t, ok)
	// The spinner tick is the last batched cmd and must produce a TickMsg so
	// the header animates while the task runs.
	tick := batch[len(batch)-1]()
	_, isTick := tick.(spinner.TickMsg)
	assert.True(t, isTick, "last batched cmd should be the spinner tick, got %T", tick)
}

func TestTaskView_ShowsSpinnerWhileRunningNotAfterDone(t *testing.T) {
	m := Model{width: 100, height: 30, phase: phaseTask, taskTitle: "Building Administration..."}
	m.dockerSpinner = newBrandSpinner()

	running := m.View().Content
	assert.Contains(t, running, "Building Administration...")

	m.taskDone = true
	done := m.View().Content
	assert.Contains(t, done, "Building Administration...")

	// The spinner glyph adds leading runes before the title only while running,
	// so the rendered task header differs between the two states.
	assert.NotEqual(t, running, done)
}

func TestRunSelfCommand_ConstructsTaskWithoutExecuting(t *testing.T) {
	m := &Model{projectRoot: t.TempDir(), dockerMode: true}

	cmd := m.runSelfCommand("Building Administration...", "project", "admin-build")
	assert.Equal(t, phaseTask, m.phase)
	assert.Equal(t, "Building Administration...", m.taskTitle)
	assert.NotNil(t, m.dockerOutChan)
	assert.NotNil(t, cmd)
}

func TestRunAdminBuild_SetsExpectedTitleAndPhase(t *testing.T) {
	m := &Model{projectRoot: t.TempDir()}
	cmd := m.runAdminBuild()
	assert.NotNil(t, cmd)
	assert.Equal(t, phaseTask, m.phase)
	assert.Equal(t, "Building Administration...", m.taskTitle)
}

func TestRunStorefrontBuild_SetsExpectedTitleAndPhase(t *testing.T) {
	m := &Model{projectRoot: t.TempDir()}
	cmd := m.runStorefrontBuild()
	assert.NotNil(t, cmd)
	assert.Equal(t, phaseTask, m.phase)
	assert.Equal(t, "Building Storefront...", m.taskTitle)
}

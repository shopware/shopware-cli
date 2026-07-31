package tui

import (
	"os/exec"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// taskLogKeep is the scrollback cap for a task's streamed output.
const taskLogKeep = 400

// TaskLineMsg carries one line of a running task's output.
type TaskLineMsg struct{ Line string }

// TaskDoneMsg is delivered when a task's command has finished and its output
// has been fully drained.
type TaskDoneMsg struct{ Err error }

// taskExitMsg carries the command's exit result. The public TaskDoneMsg is
// only emitted once the output stream has also drained — the exit result
// usually arrives while buffered lines are still queued in the channel, and
// finishing early would truncate the visible log.
type taskExitMsg struct{ err error }

// taskStreamClosedMsg signals that the output channel drained.
type taskStreamClosedMsg struct{}

// Task runs one command in the background and accumulates its streamed
// output — the shared "run job, show spinner + log tail" machinery behind
// task screens. Embed it in a model, call Start, and route TaskLineMsg,
// TaskDoneMsg, and spinner.TickMsg through Update.
type Task struct {
	Title string

	spinner spinner.Model
	lines   []string
	ch      <-chan string
	exited  bool
	drained bool
	done    bool
	err     error
}

// NewTask creates an idle task.
func NewTask(title string) Task {
	return Task{Title: title, spinner: NewBrandSpinner()}
}

// Start launches the command produced by factory and begins streaming its
// combined output. The returned command batch keeps the stream flowing and
// the spinner ticking; completion arrives as a TaskDoneMsg.
func (t *Task) Start(factory func() (*exec.Cmd, error)) tea.Cmd {
	t.lines = nil
	t.exited = false
	t.drained = false
	t.done = false
	t.err = nil

	ch := make(chan string, StreamBufferSize)
	t.ch = ch

	run := func() tea.Msg {
		cmd, err := factory()
		if err != nil {
			close(ch)
			return taskExitMsg{err: err}
		}
		return taskExitMsg{err: StreamCmdOutput(cmd, ch, true)}
	}

	// The spinner tick (kept last) keeps the title animated so long-running
	// commands that emit no early output never look frozen.
	return tea.Batch(t.readLine(), run, t.spinner.Tick)
}

func (t Task) readLine() tea.Cmd {
	return ReadLineCmd(t.ch, func(line string) tea.Msg { return TaskLineMsg{Line: line} }, taskStreamClosedMsg{})
}

// finish marks the task done and emits TaskDoneMsg once both the exit result
// arrived and the output stream drained, whichever came last. It returns the
// updated task: mutating through a pointer inside a return statement would
// copy the value before the mutation (unspecified evaluation order).
func (t Task) finish() (Task, tea.Cmd) {
	if !t.exited || !t.drained || t.done {
		return t, nil
	}
	t.done = true
	err := t.err
	return t, func() tea.Msg { return TaskDoneMsg{Err: err} }
}

// Update handles the task's stream, completion, and spinner messages.
func (t Task) Update(msg tea.Msg) (Task, tea.Cmd) {
	switch msg := msg.(type) {
	case TaskLineMsg:
		t.lines = AppendTail(t.lines, taskLogKeep, msg.Line)
		return t, t.readLine()

	case taskStreamClosedMsg:
		t.drained = true
		return t.finish()

	case taskExitMsg:
		t.exited = true
		t.err = msg.err
		return t.finish()

	case TaskDoneMsg:
		// Normally self-emitted by finish (a no-op then); also accepted from
		// the embedding model to force the final state.
		t.done = true
		t.err = msg.Err
		return t, nil

	case spinner.TickMsg:
		// Stop ticking once the task is done so the final output stays static.
		if t.done {
			return t, nil
		}
		var cmd tea.Cmd
		t.spinner, cmd = t.spinner.Update(msg)
		return t, cmd
	}
	return t, nil
}

// Done reports whether the command has finished.
func (t Task) Done() bool { return t.done }

// Err returns the command's error, nil while running or on success.
func (t Task) Err() error { return t.err }

// Lines returns the accumulated output scrollback.
func (t Task) Lines() []string { return t.lines }

// StatusTitle returns the task title, prefixed with the spinner while the
// command is still running.
func (t Task) StatusTitle() string {
	if t.done {
		return t.Title
	}
	return t.spinner.View() + " " + t.Title
}

package app

import (
	tea "charm.land/bubbletea/v2"
)

// Harness drives an App without a TTY for tests: construct with a size, Send
// messages, and assert on View output. The Init command is executed during
// construction; commands returned by later Update calls are NOT executed
// automatically — use SendCmd when a test needs the resulting message.
type Harness struct {
	App *App
}

// NewHarness builds an App from opts, runs Init, and delivers the initial
// window size.
func NewHarness(opts Options, width, height int) *Harness {
	h := &Harness{App: New(opts)}
	initCmd := h.App.Init()
	h.Send(tea.WindowSizeMsg{Width: width, Height: height})
	h.SendCmd(initCmd)
	return h
}

// Send delivers messages to the App in order and returns the last command.
func (h *Harness) Send(msgs ...tea.Msg) tea.Cmd {
	var last tea.Cmd
	for _, msg := range msgs {
		_, last = h.App.Update(msg)
	}
	return last
}

// SendCmd resolves cmd (if any) and feeds every produced message back into
// the App, following tea.Batch trees. Depth and command-count limits prevent
// recurring commands such as spinner ticks from making the harness loop
// forever.
func (h *Harness) SendCmd(cmd tea.Cmd) {
	const (
		maxDepth    = 100
		maxCommands = 10_000
	)
	type pendingCmd struct {
		cmd   tea.Cmd
		depth int
	}

	queue := []pendingCmd{{cmd: cmd}}
	commandsRun := 0
	for len(queue) > 0 && commandsRun < maxCommands {
		pending := queue[0]
		queue = queue[1:]
		if pending.cmd == nil || pending.depth >= maxDepth {
			continue
		}

		commandsRun++
		msg := pending.cmd()
		if msg == nil {
			continue
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, child := range batch {
				queue = append(queue, pendingCmd{cmd: child, depth: pending.depth + 1})
			}
			continue
		}
		if next := h.Send(msg); next != nil {
			queue = append(queue, pendingCmd{cmd: next, depth: pending.depth + 1})
		}
	}
}

// View renders the current frame content.
func (h *Harness) View() string {
	return h.App.View().Content
}

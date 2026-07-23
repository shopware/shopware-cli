package app

import (
	tea "charm.land/bubbletea/v2"
)

// Harness drives an App without a TTY for tests: construct with a size, Send
// messages, and assert on View output. Commands returned by Update are NOT
// executed automatically — use SendCmd to resolve one when a test needs the
// resulting message delivered.
type Harness struct {
	App *App
}

// NewHarness builds an App from opts, runs Init, and delivers the initial
// window size.
func NewHarness(opts Options, width, height int) *Harness {
	h := &Harness{App: New(opts)}
	_ = h.App.Init()
	h.Send(tea.WindowSizeMsg{Width: width, Height: height})
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
// the App, following tea.Batch trees. Nil commands are ignored.
func (h *Harness) SendCmd(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if msg == nil {
		return
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			h.SendCmd(c)
		}
		return
	}
	h.SendCmd(h.Send(msg))
}

// View renders the current frame content.
func (h *Harness) View() string {
	return h.App.View().Content
}

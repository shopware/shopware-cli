package upgradetui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// runState backs panel 5: live progress while the upgrade executes.
type runState struct {
	events   <-chan upgrade.StepEvent
	cancelFn context.CancelFunc

	stepStates map[upgrade.StepID]upgrade.CheckState
	stepErrs   map[upgrade.StepID]error
	logLines   []string
	fullLog    bool

	finished  bool
	succeeded bool
	err       error
}

const runLogKeep = 500

// beginRun starts the runner and switches to the progress panel.
func (m *Model) beginRun() (app.Content, tea.Cmd) {
	target := m.check.target()
	if target == nil {
		return m, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	opts := upgrade.RunOptions{
		Target: target.Version.String(),
		Report: m.reportData(),
	}
	if m.prepare.resolve != nil {
		opts.ResolvedVersions = m.prepare.resolve.VersionMap()
	}

	m.panel = panelRun
	m.run = runState{
		cancelFn:   cancel,
		stepStates: make(map[upgrade.StepID]upgrade.CheckState),
		stepErrs:   make(map[upgrade.StepID]error),
	}
	m.run.events = m.upgrader.Run(ctx, opts)

	return m, readRunEventCmd(m.run.events)
}

// cancel aborts the running upgrade; the runner rolls back and finishes.
func (s *runState) cancel() {
	if s.cancelFn != nil {
		s.cancelFn()
	}
}

func (m *Model) updateRun(msg tea.Msg) (app.Content, tea.Cmd) {
	switch msg := msg.(type) {
	case runEventMsg:
		ev := upgrade.StepEvent(msg)
		switch {
		case ev.Line != "":
			m.run.logLines = tui.AppendTail(m.run.logLines, runLogKeep, ev.Line)
		case ev.Step == upgrade.StepFinished:
			m.run.finished = true
			m.run.succeeded = ev.State == upgrade.StateOK
			m.run.err = ev.Err
		default:
			m.run.stepStates[ev.Step] = ev.State
			if ev.Err != nil {
				m.run.stepErrs[ev.Step] = ev.Err
			}
		}
		return m, readRunEventCmd(m.run.events)

	case runClosedMsg:
		return m.beginDone()

	case tea.KeyPressMsg:
		if app.KeyString(msg) == "l" {
			m.run.fullLog = !m.run.fullLog
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) viewRun() (title, status, body string) {
	title = "Upgrade in progress"

	switch {
	case m.run.finished && !m.run.succeeded:
		status = m.statusStrip(tui.VariantError, "FAILED", "The upgrade was rolled back. See the log for details.")
	case m.run.finished:
		status = m.statusStrip(tui.VariantSuccess, "DONE", "All tasks completed.")
	default:
		status = m.statusStrip(tui.VariantWarning, "RUNNING", "This may take a few minutes. Live output is shown on the right.")
	}

	if m.run.fullLog {
		body = m.viewRunLog(m.bodyWidth())
	} else {
		body = m.twoColumn(m.bodyWidth()*2/5, m.viewRunProgress(), m.viewRunLog(m.bodyWidth()*3/5-3))
	}

	return title, status, body
}

func (m *Model) viewRunProgress() string {
	var b strings.Builder
	b.WriteString(tui.BoldStyle.Render("Progress"))
	b.WriteString("\n\n")

	for _, id := range upgrade.RunSteps {
		state, seen := m.run.stepStates[id]
		if !seen {
			state = upgrade.StatePending
		}
		b.WriteString(stateDot(state) + " " + tui.LabelStyle.Render(id.Label()))
		b.WriteString("\n")
		if err := m.run.stepErrs[id]; err != nil {
			style := failStyle
			if state == upgrade.StateWarn {
				style = warnStyle
			}
			b.WriteString("   " + style.Render(tui.Truncate(err.Error(), 60)))
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m *Model) viewRunLog(width int) string {
	var b strings.Builder
	b.WriteString(tui.BoldStyle.Render("Live output"))
	b.WriteString("\n\n")

	visible := m.bodyHeight(true) - 3
	if visible < 3 {
		visible = 3
	}

	for _, line := range tui.TailLines(m.run.logLines, visible) {
		b.WriteString(tui.DimStyle.Render(tui.Truncate(line, width)))
		b.WriteString("\n")
	}
	if len(m.run.logLines) == 0 {
		b.WriteString(tui.DimStyle.Render("Waiting for output…"))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render(tui.Truncate("Full log: "+m.upgrader.LogPath(), width)))

	return b.String()
}

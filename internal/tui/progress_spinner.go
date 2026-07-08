package tui

import (
	"context"
	"os/exec"
	"strings"
	"sync"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// RunSpinnerWithLogs executes the given command while displaying a spinner and allowing the user to toggle logs with Ctrl+L.
func RunSpinnerWithLogs(ctx context.Context, title string, cmd *exec.Cmd) error {
	cmdCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var writer logWriter
	cmd.Stdout = &writer
	cmd.Stderr = &writer

	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(BrandColor)),
	)

	model := &installProgressModel{
		spinner:   s,
		logWriter: &writer,
		title:     title,
		cmd:       cmd,
		cancel:    cancel,
	}

	p := tea.NewProgram(model, tea.WithContext(cmdCtx))
	if _, err := p.Run(); err != nil {
		return err
	}

	if model.cancelled {
		return context.Canceled
	}

	return model.err
}

type logWriter struct {
	mu      sync.Mutex
	lines   []string
	current strings.Builder
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for _, b := range p {
		if b == '\n' {
			w.lines = append(w.lines, w.current.String())
			w.current.Reset()
			if len(w.lines) > 100 {
				w.lines = w.lines[len(w.lines)-100:]
			}
		} else if b != '\r' {
			w.current.WriteByte(b)
		}
	}
	return len(p), nil
}

func (w *logWriter) GetLastLines(n int) []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	var res []string
	if len(w.lines) > 0 {
		res = make([]string, len(w.lines))
		copy(res, w.lines)
	}

	if w.current.Len() > 0 {
		res = append(res, w.current.String())
	}

	if len(res) <= n {
		return res
	}
	return res[len(res)-n:]
}

type installFinishedMsg struct {
	err error
}

type installProgressModel struct {
	spinner   spinner.Model
	logWriter *logWriter
	showLogs  bool
	title     string
	cmd       *exec.Cmd
	cancel    context.CancelFunc

	width     int
	height    int
	done      bool
	cancelled bool
	err       error
}

func (m *installProgressModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		err := m.cmd.Run()
		return installFinishedMsg{err: err}
	})
}

func (m *installProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelled = true
			m.cancel()
			return m, tea.Quit
		case "ctrl+l":
			m.showLogs = !m.showLogs
			return m, nil
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case installFinishedMsg:
		m.done = true
		m.err = msg.err
		return m, tea.Quit
	}

	return m, nil
}

func (m *installProgressModel) View() tea.View {
	var b strings.Builder

	spinnerStr := m.spinner.View()
	titleStyle := lipgloss.NewStyle().Bold(true)

	if m.done {
		if m.err != nil {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4D4D")).Bold(true).Render("✗") + " " + titleStyle.Render(m.title) + "\n\n")

			lines := m.logWriter.GetLastLines(12)
			logStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4D4D")).PaddingLeft(2)
			for _, line := range lines {
				b.WriteString(logStyle.Render(line) + "\n")
			}
		} else {
			b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true).Render("✔") + " " + titleStyle.Render(m.title) + "\n")
		}
		return tea.NewView(b.String())
	}

	var hint string
	if m.showLogs {
		hint = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(" (Ctrl+L to hide live log)")
	} else {
		hint = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render(" (Ctrl+L to see live log)")
	}

	b.WriteString(spinnerStr + " " + titleStyle.Render(m.title) + hint + "\n")

	if m.showLogs {
		b.WriteString("\n")
		lines := m.logWriter.GetLastLines(8)

		var logBody strings.Builder
		for i, line := range lines {
			logBody.WriteString(line)
			if i < len(lines)-1 {
				logBody.WriteString("\n")
			}
		}
		if len(lines) == 0 {
			logBody.WriteString("Waiting for output...")
		}

		width := m.width - 4
		if width < 40 {
			width = 78
		}
		if width > 100 {
			width = 100
		}

		borderStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#303030")).
			Padding(0, 1).
			Width(width)

		b.WriteString(borderStyle.Render(logBody.String()))
		b.WriteString("\n")
	}

	return tea.NewView(b.String())
}

package project

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/shopware/shopware-cli/internal/tui"
)

const (
	// installLogVisibleLines is the number of trailing output lines shown in the
	// live log panel while it is toggled on.
	installLogVisibleLines = 8
	// installLogStreamBuffer bounds the channel that buffers command output
	// lines between the streaming goroutine and the Bubble Tea update loop.
	installLogStreamBuffer = 64
)

// installLineMsg is emitted for every line of combined command output.
type installLineMsg string

// installDoneMsg is emitted once the command finished. output holds the full
// captured output so it can be printed when the command failed.
type installDoneMsg struct {
	err    error
	output []string
}

// installLogModel renders a spinner while a command runs and lets the user
// toggle a live log tail with "l" (or CTRL+L). It powers the interactive
// "Installing dependencies" step of `project create`, giving feedback on the
// long-running Composer install without flooding the terminal by default.
type installLogModel struct {
	title   string
	spinner spinner.Model
	width   int

	cmd *exec.Cmd
	ch  chan string

	lines   []string
	showLog bool

	done   bool
	runErr error
	output []string
}

func newInstallLogModel(title string, cmd *exec.Cmd) installLogModel {
	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(tui.BrandColor)),
	)

	return installLogModel{
		title:   title,
		spinner: s,
		width:   tui.TerminalWidth(),
		cmd:     cmd,
		ch:      make(chan string, installLogStreamBuffer),
	}
}

func (m installLogModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.readNextLine(), m.runCommand())
}

// runCommand streams the command's combined output into the channel and reports
// the full output plus the exit error once it finished.
func (m installLogModel) runCommand() tea.Cmd {
	cmd := m.cmd
	ch := m.ch
	return func() tea.Msg {
		output, err := streamCombinedOutput(cmd, ch)
		return installDoneMsg{err: err, output: output}
	}
}

// readNextLine waits for the next output line. It returns nil once the channel
// is closed, which stops the read loop.
func (m installLogModel) readNextLine() tea.Cmd {
	ch := m.ch
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return installLineMsg(line)
	}
}

func (m installLogModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "l", "ctrl+l":
			m.showLog = !m.showLog
		case "ctrl+c":
			return m, tea.Interrupt
		}
		return m, nil

	case installLineMsg:
		m.lines = append(m.lines, string(msg))
		return m, m.readNextLine()

	case installDoneMsg:
		m.done = true
		m.runErr = msg.err
		m.output = msg.output
		return m, tea.Quit

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m installLogModel) View() tea.View {
	// Collapse the inline area once finished so the surrounding log output
	// continues cleanly where the spinner used to be.
	if m.done {
		return tea.NewView("")
	}

	hint := "show live log"
	if m.showLog {
		hint = "hide live log"
	}

	line := fmt.Sprintf("%s %s   %s", m.spinner.View(), m.title, tui.DimStyle.Render("press l to "+hint))

	if !m.showLog {
		return tea.NewView(line)
	}

	return tea.NewView(line + "\n" + m.renderLogPanel())
}

func (m installLogModel) renderLogPanel() string {
	width := m.width
	if width <= 0 {
		width = tui.TerminalWidth()
	}

	// Account for the rounded border (1 cell each side) and the horizontal
	// padding (1 cell each side) so lines are truncated instead of wrapped,
	// which keeps the panel height stable.
	const borderAndPadding = 4
	inner := width - borderAndPadding
	if inner < 8 {
		inner = 8
	}

	lines := m.lines
	if len(lines) > installLogVisibleLines {
		lines = lines[len(lines)-installLogVisibleLines:]
	}

	var body strings.Builder
	body.WriteString(tui.DimStyle.Render("Live log"))
	body.WriteString("\n")
	if len(lines) == 0 {
		body.WriteString(tui.DimStyle.Render("Waiting for output…"))
	} else {
		for i, l := range lines {
			if i > 0 {
				body.WriteString("\n")
			}
			body.WriteString(ansi.Truncate(l, inner, "…"))
		}
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BorderColor).
		Padding(0, 1).
		Width(width).
		Render(body.String())
}

// runInstallWithLiveLog runs cmd while showing a spinner with a toggleable live
// log tail. On failure the captured output is printed to stderr so the error is
// not swallowed by the spinner UI.
func runInstallWithLiveLog(ctx context.Context, title string, cmd *exec.Cmd) error {
	finalModel, err := tea.NewProgram(newInstallLogModel(title, cmd), tea.WithContext(ctx)).Run()
	if err != nil {
		return err
	}

	final, ok := finalModel.(installLogModel)
	if !ok {
		return nil
	}

	if final.runErr != nil {
		if len(final.output) > 0 {
			fmt.Fprintln(os.Stderr, strings.Join(final.output, "\n"))
		}
		return final.runErr
	}

	return nil
}

// streamCombinedOutput starts cmd with stdout and stderr merged, forwarding each
// line to ch (for the live view) while also collecting the full output (for
// error reporting). The channel is always closed before returning.
func streamCombinedOutput(cmd *exec.Cmd, ch chan<- string) ([]string, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		close(ch)
		return nil, err
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		close(ch)
		return nil, err
	}

	var lines []string
	scanner := bufio.NewScanner(stdout)
	// Composer and, in Docker mode, image pulls can emit long progress lines.
	// Allow up to 1 MiB per line so a long line is not mistaken for a scan
	// error (which would wrongly surface as an install failure).
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		lines = append(lines, line)
		ch <- line
	}
	close(ch)

	waitErr := cmd.Wait()
	if scanErr := scanner.Err(); scanErr != nil && waitErr == nil {
		waitErr = scanErr
	}

	return lines, waitErr
}

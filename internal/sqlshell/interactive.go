package sqlshell

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// InteractiveShell runs the read-eval-print loop as an inline bubbletea
// program, providing real line editing (word deletion via alt+backspace,
// ctrl+w and alt+d, cursor movement) and session history on the arrow keys.
// Results are printed into the normal terminal scrollback.
func InteractiveShell(ctx context.Context, db Conn, format Format) error {
	_, err := tea.NewProgram(newInteractiveModel(ctx, db, format), tea.WithContext(ctx)).Run()

	return filterInteractiveErr(err)
}

// filterInteractiveErr treats context cancellation as a normal shell exit;
// bubbletea wraps it in tea.ErrProgramKilled.
func filterInteractiveErr(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}

func newInteractiveModel(ctx context.Context, db Conn, format Format) *interactiveModel {
	input := textinput.New()
	input.Prompt = promptMain
	input.Focus()

	return &interactiveModel{
		newExecution: func(statements []string, quit bool) (context.CancelFunc, tea.Cmd) {
			runCtx, cancel := context.WithCancel(ctx)

			return cancel, func() tea.Msg {
				var out bytes.Buffer

				for _, stmt := range statements {
					err := RunStatement(runCtx, db, stmt, &out, format)

					if runCtx.Err() != nil {
						// Cancelling closes the driver connection mid-query,
						// the session connection may not survive it.
						_, _ = fmt.Fprintln(&out, "Query cancelled. Restart the shell if further statements fail.")
						break
					}

					if err != nil {
						_, _ = fmt.Fprintf(&out, "ERROR: %s\n", err)
					}
				}

				return resultMsg{output: strings.TrimRight(out.String(), "\n"), quit: quit}
			}
		},
		input:     input,
		delimiter: DefaultDelimiter,
	}
}

// resultMsg carries the rendered output of executed statements back into the
// update loop.
type resultMsg struct {
	output string
	quit   bool
}

type interactiveModel struct {
	// newExecution prepares one asynchronous statement run against the
	// session connection; it carries the command context in its closure and
	// returns the cancel function for that run.
	newExecution func(statements []string, quit bool) (context.CancelFunc, tea.Cmd)

	input     textinput.Model
	buffer    string
	delimiter string

	history []string
	histPos int
	stashed string

	running    bool
	cancelling bool
	cancelRun  context.CancelFunc
	done       bool
}

func (m *interactiveModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *interactiveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.input.SetWidth(max(msg.Width-len(promptContinuation)-1, 20))
		return m, nil

	case resultMsg:
		m.running = false
		m.cancelling = false

		if m.cancelRun != nil {
			m.cancelRun()
			m.cancelRun = nil
		}

		var cmds []tea.Cmd
		if msg.output != "" {
			cmds = append(cmds, tea.Println(msg.output))
		}

		if msg.quit {
			m.done = true
			cmds = append(cmds, tea.Quit)
		}

		return m, tea.Sequence(cmds...)

	case tea.KeyPressMsg:
		if m.running {
			// The terminal is in raw mode, ctrl+c raises no SIGINT: cancel
			// the running statement here instead.
			if msg.String() == "ctrl+c" && m.cancelRun != nil && !m.cancelling {
				m.cancelling = true
				m.cancelRun()
			}

			return m, nil
		}

		return m.handleKey(msg)
	}

	return m, nil
}

func (m *interactiveModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return m.submitLine()

	case "up":
		if m.histPos > 0 {
			if m.histPos == len(m.history) {
				m.stashed = m.input.Value()
			}
			m.histPos--
			m.input.SetValue(m.history[m.histPos])
			m.input.CursorEnd()
		}
		return m, nil

	case "down":
		if m.histPos < len(m.history) {
			m.histPos++
			if m.histPos == len(m.history) {
				m.input.SetValue(m.stashed)
			} else {
				m.input.SetValue(m.history[m.histPos])
			}
			m.input.CursorEnd()
		}
		return m, nil

	case "ctrl+c":
		// Drop the pending input like the mysql client; quit when idle.
		if m.input.Value() == "" && m.buffer == "" {
			m.done = true
			return m, tea.Quit
		}

		echo := tea.Println(m.prompt() + m.input.Value() + "^C")
		m.reset()
		return m, echo

	case "ctrl+d":
		// End of input on an empty line: run what is pending, then quit.
		if m.input.Value() == "" {
			return m.finish()
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)

	return m, cmd
}

// submitLine feeds the entered line into the statement buffer and executes
// every completed statement.
func (m *interactiveModel) submitLine() (tea.Model, tea.Cmd) {
	line := m.input.Value()
	echo := tea.Println(m.prompt() + line)

	if strings.TrimSpace(line) != "" {
		m.history = append(m.history, line)
	}
	m.histPos = len(m.history)
	m.stashed = ""
	m.input.SetValue("")

	if m.buffer == "" && isExitCommand(line) {
		m.done = true
		return m, tea.Sequence(echo, tea.Quit)
	}

	m.buffer += line + "\n"

	var statements []string
	statements, m.buffer, m.delimiter = SplitStatementsWithDelimiter(m.buffer, m.delimiter)

	m.input.Prompt = m.prompt()

	if len(statements) == 0 {
		return m, echo
	}

	return m, tea.Sequence(echo, m.startExecution(statements, false))
}

// finish runs a trailing statement without semicolon (matching Run and
// ExecuteStream) and quits.
func (m *interactiveModel) finish() (tea.Model, tea.Cmd) {
	if rest := strings.TrimSpace(m.buffer); rest != "" {
		m.buffer = ""

		return m, m.startExecution([]string{rest}, true)
	}

	m.done = true

	return m, tea.Quit
}

// startExecution kicks off an asynchronous, cancellable statement run.
// Statement errors are printed, they do not end the session.
func (m *interactiveModel) startExecution(statements []string, quit bool) tea.Cmd {
	cancel, cmd := m.newExecution(statements, quit)

	m.cancelRun = cancel
	m.running = true

	return cmd
}

func (m *interactiveModel) prompt() string {
	if m.buffer != "" {
		return promptContinuation
	}

	return promptMain
}

func (m *interactiveModel) reset() {
	m.buffer = ""
	m.histPos = len(m.history)
	m.stashed = ""
	m.input.SetValue("")
	m.input.Prompt = promptMain
}

func (m *interactiveModel) View() tea.View {
	if m.done {
		return tea.NewView("")
	}

	if m.running {
		if m.cancelling {
			return tea.NewView("Cancelling query...")
		}

		return tea.NewView("Executing... press ctrl+c to cancel")
	}

	return tea.NewView(m.input.View())
}

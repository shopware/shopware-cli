package devtui

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

func newBrandSpinner() spinner.Model {
	return spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(tui.BrandColor)),
	)
}

func newInstallProgress() progress.Model {
	return progress.New(
		progress.WithColors(tui.BrandColor),
		progress.WithWidth(tui.PhaseCardWidth-15),
		progress.WithoutPercentage(),
	)
}

func checkContainersRunning(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		check := exec.CommandContext(ctx, "docker", "compose", "ps", "--status=running", "-q")
		check.Dir = projectRoot
		output, err := check.Output()
		if err == nil && len(strings.TrimSpace(string(output))) > 0 {
			return dockerAlreadyRunningMsg{}
		}
		return dockerNeedStartMsg{}
	}
}

func (m *Model) checkShopwareInstalled() tea.Cmd {
	exec := m.executor
	return func() tea.Msg {
		cmd := exec.ConsoleCommand(context.Background(), "system:is-installed")
		if err := cmd.Run(); err != nil {
			return shopwareNotInstalledMsg{}
		}
		return shopwareInstalledMsg{}
	}
}

func (m *Model) runShopwareInstall() tea.Cmd {
	e := m.executor
	language := m.install.language
	currency := m.install.currency
	username := m.install.username.Value()
	password := m.install.password.Value()

	ch := make(chan string, streamBufferSize)
	m.dockerOutChan = ch

	doneCmd := func() tea.Msg {
		withEnv := e.WithEnv(map[string]string{
			"INSTALL_LOCALE":         language,
			"INSTALL_CURRENCY":       currency,
			"INSTALL_ADMIN_USERNAME": username,
			"INSTALL_ADMIN_PASSWORD": password,
		})
		cmd := withEnv.PHPCommand(context.Background(), "vendor/bin/shopware-deployment-helper", "run")

		err := streamCmdOutput(cmd, ch, true)
		return shopwareInstallDoneMsg{err: err}
	}

	return tea.Batch(readFromChan(ch), doneCmd)
}

func (m *Model) readNextDockerOutput() tea.Cmd {
	ch := m.dockerOutChan
	if ch == nil {
		return nil
	}
	return readFromChan(ch)
}

// streamBufferSize is the channel buffer size used for streaming command output.
const streamBufferSize = 50

// readFromChan returns a tea.Cmd that reads one line from ch and produces
// a dockerOutputLineMsg, or dockerOutputDoneMsg when the channel is closed.
func readFromChan(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return dockerOutputDoneMsg{}
		}
		return dockerOutputLineMsg(line)
	}
}

// streamCmdOutput starts cmd, scans its output line by line into ch, then
// closes ch. If useStdout is true it pipes stdout (merging stderr into it);
// otherwise it pipes stderr (merging stdout into it).
// Returns the error from cmd.Wait.
func streamCmdOutput(cmd *exec.Cmd, ch chan<- string, useStdout bool) error {
	var pipe io.Reader
	var err error
	if useStdout {
		pipe, err = cmd.StdoutPipe()
		if err == nil {
			cmd.Stderr = cmd.Stdout
		}
	} else {
		pipe, err = cmd.StderrPipe()
		if err == nil {
			cmd.Stdout = cmd.Stderr
		}
	}
	if err != nil {
		close(ch)
		return err
	}

	if err := cmd.Start(); err != nil {
		close(ch)
		return err
	}

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		ch <- scanner.Text()
	}
	close(ch)

	return cmd.Wait()
}

// runDockerCommandWithArgs runs a docker compose command, streaming stderr lines
// through a channel for display, and returns a result message when done.
func runDockerCommandWithArgs(ctx context.Context, projectRoot string, args []string, resultFn func(error) tea.Msg) (outChan <-chan string, outputCmd tea.Cmd, doneCmd tea.Cmd) {
	lineChan := make(chan string, streamBufferSize)

	doneCmd = func() tea.Msg {
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Dir = projectRoot
		return resultFn(streamCmdOutput(cmd, lineChan, false))
	}

	return lineChan, readFromChan(lineChan), doneCmd
}

func (m *Model) startContainers() tea.Cmd {
	ch, outputCmd, doneCmd := runDockerCommandWithArgs(
		context.Background(),
		m.projectRoot,
		[]string{"compose", "up", "-d"},
		func(err error) tea.Msg { return dockerStartedMsg{err: err} },
	)
	m.dockerOutChan = ch
	return tea.Batch(outputCmd, doneCmd)
}

func (m *Model) stopContainers() tea.Cmd {
	ch, outputCmd, doneCmd := runDockerCommandWithArgs(
		context.Background(),
		m.projectRoot,
		[]string{"compose", "down"},
		func(err error) tea.Msg { return dockerStoppedMsg{err: err} },
	)
	m.dockerOutChan = ch
	return tea.Batch(outputCmd, doneCmd)
}

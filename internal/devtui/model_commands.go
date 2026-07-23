package devtui

import (
	"context"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/tui"
)

func newInstallProgress() progress.Model {
	return progress.New(
		progress.WithColors(tui.BrandColor),
		progress.WithWidth(tui.PhaseCardWidth-15),
		progress.WithoutPercentage(),
	)
}

func checkContainersRunning(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		running := composeServiceSet(projectRoot, "ps", "--services", "--status=running")
		if len(running) == 0 {
			return dockerNeedStartMsg{}
		}

		// Treat the stack as already up only when every service the compose
		// file defines is running. A service that was just added to
		// compose.yaml (e.g. the messenger worker when the admin worker is
		// disabled) is not running yet, so fall through to a start and let
		// `up -d` reconcile the newcomers instead of jumping to the dashboard.
		defined := composeServiceSet(projectRoot, "config", "--services")
		if !allRunning(defined, running) {
			return dockerNeedStartMsg{}
		}

		return dockerAlreadyRunningMsg{}
	}
}

// allRunning reports whether every service in defined is present in running.
// An empty defined set (e.g. when the compose config could not be read) imposes
// no constraint and is considered satisfied.
func allRunning(defined, running map[string]struct{}) bool {
	for name := range defined {
		if _, ok := running[name]; !ok {
			return false
		}
	}

	return true
}

// composeServiceSet runs a docker compose command that prints one service name
// per line (e.g. `config --services` or `ps --services`) and returns the names
// as a set. It returns nil when the command fails, so callers treat an
// undeterminable list as "no constraint".
func composeServiceSet(projectRoot string, args ...string) map[string]struct{} {
	output, err := composeCommand(context.Background(), projectRoot, args...).Output()
	if err != nil {
		return nil
	}

	set := map[string]struct{}{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			set[name] = struct{}{}
		}
	}

	return set
}

func (m *Model) checkShopwareInstalled() tea.Cmd {
	exec := m.executor
	return func() tea.Msg {
		if err := exec.ConsoleCommand(context.Background(), "system:is-installed").Run(); err != nil {
			return shopwareNotInstalledMsg{}
		}
		return shopwareInstalledMsg{}
	}
}

func (m *Model) runShopwareInstall() tea.Cmd {
	e := m.executor
	language := m.install.language
	currency := m.install.currency
	username := m.install.Username()
	password := m.install.Password()

	ch := make(chan string, tui.StreamBufferSize)
	m.dockerOutChan = ch

	doneCmd := func() tea.Msg {
		withEnv := e.WithEnv(map[string]string{
			"INSTALL_LOCALE":         language,
			"INSTALL_CURRENCY":       currency,
			"INSTALL_ADMIN_USERNAME": username,
			"INSTALL_ADMIN_PASSWORD": password,
		})
		p := withEnv.PHPCommand(context.Background(), "vendor/bin/shopware-deployment-helper", "run")

		err := streamCmdOutput(p.Cmd, ch, true)
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

func readFromChan(ch <-chan string) tea.Cmd {
	return tui.ReadLineCmd(ch,
		func(line string) tea.Msg { return dockerOutputLineMsg(line) },
		dockerOutputDoneMsg{},
	)
}

func streamCmdOutput(cmd *exec.Cmd, ch chan<- string, useStdout bool) error {
	return tui.StreamCmdOutput(cmd, ch, useStdout)
}

func runComposeCommand(ctx context.Context, projectRoot string, args []string, resultFn func(error) tea.Msg) (outChan <-chan string, outputCmd tea.Cmd, doneCmd tea.Cmd) {
	lineChan := make(chan string, tui.StreamBufferSize)

	doneCmd = func() tea.Msg {
		cmd := composeCommand(ctx, projectRoot, args...)
		return resultFn(streamCmdOutput(cmd, lineChan, false))
	}

	return lineChan, readFromChan(lineChan), doneCmd
}

func (m *Model) startContainers() tea.Cmd {
	m.telemetry.beginDockerStart()
	ch, outputCmd, doneCmd := runComposeCommand(
		context.Background(),
		m.projectRoot,
		[]string{"up", "-d"},
		func(err error) tea.Msg { return dockerStartedMsg{err: err} },
	)
	m.dockerOutChan = ch
	return tea.Batch(outputCmd, doneCmd)
}

func (m *Model) restartContainersForConfig() tea.Cmd {
	m.telemetry.beginConfigRestart()
	projectRoot := m.projectRoot
	cfg := m.config
	return func() tea.Msg {
		if err := dockerpkg.WriteComposeFile(projectRoot, dockerpkg.ComposeOptionsFromConfig(cfg)); err != nil {
			return configRestartDoneMsg{err: err}
		}
		cmd := composeCommand(context.Background(), projectRoot, "up", "-d")
		return configRestartDoneMsg{err: cmd.Run()}
	}
}

func (m *Model) stopContainers() tea.Cmd {
	ch, outputCmd, doneCmd := runComposeCommand(
		context.Background(),
		m.projectRoot,
		[]string{"down"},
		func(err error) tea.Msg { return dockerStoppedMsg{err: err} },
	)
	m.dockerOutChan = ch
	return tea.Batch(outputCmd, doneCmd)
}

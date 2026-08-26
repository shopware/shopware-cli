package dev

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/shop/install"
	"github.com/shopware/shopware-cli/internal/tui"
)

func newInstallProgress() progress.Model {
	return progress.New(
		progress.WithColors(tui.BrandColor),
		progress.WithWidth(tui.PhaseCardWidth-15),
		progress.WithoutPercentage(),
	)
}

func checkContainersRunning(ctx context.Context, projectRoot string) tea.Cmd {
	return func() tea.Msg {
		running := composeServiceSet(ctx, projectRoot, "ps", "--services", "--status=running")
		if len(running) == 0 {
			return dockerNeedStartMsg{}
		}

		// Treat the stack as already up only when every service the compose
		// file defines is running. A service that was just added to
		// compose.yaml (e.g. the messenger worker when the admin worker is
		// disabled) is not running yet, so fall through to a start and let
		// `up -d` reconcile the newcomers instead of jumping to the dashboard.
		defined := composeServiceSet(ctx, projectRoot, "config", "--services")
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
func composeServiceSet(ctx context.Context, projectRoot string, args ...string) map[string]struct{} {
	output, err := composeCommand(ctx, projectRoot, args...).Output()
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
	ctx := m.commandContext()
	exec := m.executor
	return func() tea.Msg {
		if !install.IsInstalled(ctx, exec) {
			return shopwareNotInstalledMsg{}
		}
		return shopwareInstalledMsg{}
	}
}

func (m *Model) runShopwareInstallFrom(fromStep string) tea.Cmd {
	ctx := m.commandContext()
	e := m.executor
	language := m.install.language
	currency := m.install.currency
	username := m.install.Username()
	password := m.install.Password()
	shopURL := m.installShopURL()
	wizard := m.install

	ch := make(chan string, tui.StreamBufferSize)
	m.dockerOutChan = ch

	doneCmd := func() tea.Msg {
		withEnv := e.WithEnv(map[string]string{
			"INSTALL_LOCALE":         language,
			"INSTALL_CURRENCY":       currency,
			"INSTALL_ADMIN_USERNAME": username,
			"INSTALL_ADMIN_PASSWORD": password,
		})

		output, err := streamInstallFrom(ctx, withEnv, fromStep, wizard, shopURL, ch)
		return shopwareInstallDoneMsg{output: output, err: err}
	}

	return tea.Batch(readFromChan(ch), doneCmd)
}

func (m Model) installShopURL() string {
	if m.envConfig != nil && m.envConfig.URL != "" {
		return strings.TrimRight(m.envConfig.URL, "/")
	}
	if m.config != nil && m.config.URL != "" {
		return strings.TrimRight(m.config.URL, "/")
	}
	return strings.TrimRight(m.overview.shopURL, "/")
}

// streamInstallFrom runs the remaining install console commands from fromStep,
// then the deployment helper so plugin/app work still happens. A failure at
// system:install (or before any step) is just the helper, matching a full run.
func streamInstallFrom(ctx context.Context, execr executor.Executor, fromStep string, w installWizard, shopURL string, ch chan<- string) ([]string, error) {
	defer close(ch)

	var all []string
	emit := func(line string) {
		all = append(all, line)
		ch <- line
	}

	idx := installStepIndex(fromStep)
	if idx > 0 {
		for _, sp := range install.Steps[idx:] {
			args := argsForInstallStep(sp.Pattern, w, shopURL)
			if len(args) == 0 {
				continue
			}
			emit("Start: " + sp.Pattern)
			lines, err := tui.DrainCmdOutput(execr.ConsoleCommand(ctx, args...).Cmd, ch, true)
			all = append(all, lines...)
			if err != nil {
				return all, err
			}
		}
	}

	lines, err := tui.DrainCmdOutput(execr.PHPCommand(ctx, "vendor/bin/shopware-deployment-helper", "run").Cmd, ch, true)
	all = append(all, lines...)
	return all, err
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

func runComposeCommand(ctx context.Context, projectRoot string, args []string, resultFn func(error) tea.Msg) (outChan <-chan string, outputCmd tea.Cmd, doneCmd tea.Cmd) {
	lineChan := make(chan string, tui.StreamBufferSize)

	doneCmd = func() tea.Msg {
		cmd := composeCommand(ctx, projectRoot, args...)
		return resultFn(tui.StreamCmdOutput(cmd, lineChan, false))
	}

	return lineChan, readFromChan(lineChan), doneCmd
}

func (m *Model) startContainers() tea.Cmd {
	m.telemetry.beginDockerStart()
	ch, outputCmd, doneCmd := runComposeCommand(
		m.commandContext(),
		m.projectRoot,
		[]string{"up", "-d"},
		func(err error) tea.Msg { return dockerStartedMsg{err: err} },
	)
	m.dockerOutChan = ch
	return tea.Batch(outputCmd, doneCmd)
}

func (m *Model) restartContainersForConfig() tea.Cmd {
	m.telemetry.beginConfigRestart()
	ctx := m.commandContext()
	projectRoot := m.projectRoot
	cfg := m.config
	return func() tea.Msg {
		if err := proxy.WriteComposeFile(projectRoot, cfg); err != nil {
			return configRestartDoneMsg{err: err}
		}
		cmd := composeCommand(ctx, projectRoot, "up", "-d")
		return configRestartDoneMsg{err: cmd.Run()}
	}
}

func (m *Model) stopContainers() tea.Cmd {
	// Stopping happens on the way out — it must still work when the command
	// context was already cancelled by a signal.
	ch, outputCmd, doneCmd := runComposeCommand(
		m.cleanupContext(),
		m.projectRoot,
		[]string{"down"},
		func(err error) tea.Msg { return dockerStoppedMsg{err: err} },
	)
	m.dockerOutChan = ch
	return tea.Batch(outputCmd, doneCmd)
}

package dev

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/shop"
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

func (m *Model) checkContainersRunning() tea.Cmd {
	ctx := m.commandContext()
	projectRoot := m.projectRoot
	return func() tea.Msg {
		running := composeServiceSet(ctx, projectRoot, "ps", "--services", "--status=running")
		if len(running) == 0 {
			return m.needStartMsg()
		}

		// Treat the stack as already up only when every service the compose
		// file defines is running. A service that was just added to
		// compose.yaml (e.g. the messenger worker when the admin worker is
		// disabled) is not running yet, so fall through to a start and let
		// `up -d` reconcile the newcomers instead of jumping to the dashboard.
		defined := composeServiceSet(ctx, projectRoot, "config", "--services")
		if !allRunning(defined, running) {
			return m.needStartMsg()
		}

		return dockerAlreadyRunningMsg{}
	}
}

// checkPortsThenStart probes for host-port conflicts and requests a container
// start when none are found.
func (m *Model) checkPortsThenStart() tea.Cmd {
	return func() tea.Msg {
		return m.needStartMsg()
	}
}

// needStartMsg probes the host ports the compose file will publish before a
// container start. Proxy-mode projects publish no host ports, so probing is
// skipped there — unless the shared proxy fell back to fixed-port mode, which
// publishes ports again. Ports held by the project's own (partially) running
// stack are not conflicts; probe errors are ignored so `docker compose up`
// surfaces real failures itself.
func (m *Model) needStartMsg() tea.Msg {
	if proxy.IsProxyProject(m.config) && !m.proxyFallback {
		return dockerNeedStartMsg{}
	}

	conflicts, err := dockerpkg.FindPortConflicts(m.commandContext(), m.projectRoot, m.dockerPorts())
	if err == nil && len(conflicts) > 0 {
		return portConflictMsg{conflicts: conflicts}
	}

	return dockerNeedStartMsg{}
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

func (m *Model) runShopwareInstall() tea.Cmd {
	ctx := m.commandContext()
	e := m.executor
	opts := install.Options{
		Locale:        m.install.language,
		Currency:      m.install.currency,
		AdminUsername: m.install.Username(),
		AdminPassword: m.install.Password(),
	}

	ch := make(chan string, tui.StreamBufferSize)
	m.dockerOutChan = ch

	doneCmd := func() tea.Msg {
		err := install.Run(ctx, e, opts, func(line string) { ch <- line })
		close(ch)
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

// fixPortConflicts allocates a random free host port for every conflicting
// port, rewrites compose.yaml and then persists the overrides to the local
// config override file. All mutations of the shared model config happen on
// the update thread after both writes succeeded, so a failed write cannot
// leave compose.yaml, the local override and the in-memory config diverged.
func (m *Model) fixPortConflicts() tea.Cmd {
	conflicts := m.portConflicts
	cfg := m.config
	projectRoot := m.projectRoot
	configPath := m.configPath
	proxyFallback := m.proxyFallback
	ctx := m.commandContext()
	return func() tea.Msg {
		overrides, err := dockerpkg.AllocateRandomPorts(ctx, conflicts)
		if err != nil {
			return portFixDoneMsg{err: err}
		}

		// Apply the overrides to a detached copy: this goroutine must not
		// mutate the shared config the UI reads concurrently, and
		// SetDockerPortOverrides merges into the Ports map in place, so the
		// map must be copied too.
		base := cfg
		if base == nil {
			base = &shop.Config{}
		}
		cfgCopy := *base
		if base.Docker != nil {
			dockerCopy := *base.Docker
			if base.Docker.Ports != nil {
				dockerCopy.Ports = make(shop.ConfigDockerPorts, len(base.Docker.Ports))
				for key, port := range base.Docker.Ports {
					dockerCopy.Ports[key] = port
				}
			}
			cfgCopy.Docker = &dockerCopy
		}
		cfgCopy.SetDockerPortOverrides(overrides)

		// A fallen-back proxy project keeps its proxy URL in the config, so
		// proxy.WriteComposeFile would regenerate the compose file in proxy
		// mode (no published ports) and silently undo the fallback; write the
		// fixed-port compose file directly instead.
		if proxyFallback {
			if err := dockerpkg.WriteComposeFile(projectRoot, dockerpkg.ComposeOptionsFromConfig(&cfgCopy)); err != nil {
				return portFixDoneMsg{err: err}
			}
		} else if err := proxy.WriteComposeFile(projectRoot, &cfgCopy); err != nil {
			return portFixDoneMsg{err: err}
		}

		if err := shop.UpdateLocalDockerPorts(configPath, overrides); err != nil {
			return portFixDoneMsg{err: err}
		}

		return portFixDoneMsg{overrides: overrides}
	}
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

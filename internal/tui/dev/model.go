package dev

import (
	"context"
	"os"
	"os/exec"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tracking"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/internal/tui/app"
	"github.com/shopware/shopware-cli/internal/tui/picker"
	"github.com/shopware/shopware-cli/internal/tui/prompt"
	"github.com/shopware/shopware-cli/internal/tui/textprompt"
)

type activeTab int

const (
	tabOverview activeTab = iota
	tabInstance
	tabConfig
)

var tabNames = []string{"Overview", "Instance", "Config"}

const (
	defaultUsername = "admin"

	watcherAdmin      = "Admin Watcher"
	watcherStorefront = "Storefront Watcher"
)

type phase int

const (
	phaseDashboard phase = iota
	phaseStarting
	phaseStopping
	phaseInstallPrompt
	phaseInstalling
	phaseTask
	phaseMigrationWizard
)

type Options struct {
	ProjectRoot string
	Config      *shop.Config
	EnvConfig   *shop.EnvironmentConfig
	Executor    executor.Executor
	// ProxyFellBack is set when a proxy project could not start the shared
	// proxy and dev fell back to fixed host ports. The shop is then reachable
	// at the local port URL, not the (now unrouted) proxy hostname in Config.
	ProxyFellBack bool
}

// fallbackShopURL is the URL a proxy project is reachable at once dev falls
// back to fixed host ports; it matches project dev's own default.
const fallbackShopURL = "http://127.0.0.1:8000"

type Model struct {
	host            app.Host
	header          tui.Header
	activeTab       activeTab
	overview        OverviewModel
	instance        InstanceModel
	configTab       ConfigModel
	width           int
	height          int
	dockerMode      bool
	phase           phase
	overlayLines    []string
	projectRoot     string
	executor        executor.Executor
	dockerOutChan   <-chan string
	install         installWizard
	installProg     installProgress
	dockerSpinner   spinner.Model
	dockerShowLogs  bool
	config          *shop.Config
	envConfig       *shop.EnvironmentConfig
	task            tui.Task
	watchers        map[string]*watcherHandle
	migrationWizard migrationWizard
	telemetry       *telemetryState
	proxyFellBack   bool
}

type dockerAlreadyRunningMsg struct{}
type dockerNeedStartMsg struct{}
type dockerStartedMsg struct{ err error }
type dockerStoppedMsg struct{ err error }
type dockerOutputLineMsg string
type dockerOutputDoneMsg struct{}

type shopwareInstalledMsg struct{}
type shopwareNotInstalledMsg struct{}
type shopwareInstallDoneMsg struct{ err error }

type configRestartDoneMsg struct{ err error }

func New(opts Options) Model {
	m := Model{
		header:        tui.NewHeader(),
		activeTab:     tabOverview,
		dockerMode:    opts.Executor.Type() == executor.TypeDocker,
		projectRoot:   opts.ProjectRoot,
		executor:      opts.Executor,
		config:        opts.Config,
		envConfig:     opts.EnvConfig,
		watchers:      make(map[string]*watcherHandle),
		telemetry:     newTelemetryState(opts.Executor.Type() == executor.TypeDocker),
		proxyFellBack: opts.ProxyFellBack,
	}
	m.rebuildTabs()
	return m
}

// rebuildTabs (re)creates the three tab models from the model's current
// config, environment config, and executor. It is used both at construction
// and after the migration wizard resolves a fresh environment, so the
// shop URL / admin credential resolution lives in one place.
func (m *Model) rebuildTabs() {
	effectiveAdminApi := m.config.AdminApi
	if m.envConfig.AdminApi != nil {
		effectiveAdminApi = m.envConfig.AdminApi
	}

	shopURL := m.config.URL
	if m.envConfig.URL != "" {
		shopURL = m.envConfig.URL
	}
	if m.proxyFellBack {
		// The proxy hostname no longer routes; the shop is on a local port.
		shopURL = fallbackShopURL
	}

	var username, password string
	if effectiveAdminApi != nil {
		username = effectiveAdminApi.Username
		password = effectiveAdminApi.Password
	}

	isDocker := m.executor.Type() == executor.TypeDocker
	envValues, _ := envfile.ReadValues(m.projectRoot, EnvFieldKeys()...)

	m.overview = NewOverviewModel(m.executor.Type(), shopURL, username, password, m.projectRoot, m.executor, m.config)
	m.instance = NewInstanceModel(m.projectRoot, isDocker)
	m.configTab = NewConfigModel(m.config, envValues)
}

// NewMigrationWizard creates a Model that starts in the migration wizard phase
// for projects that don't yet have a development environment configured.
func NewMigrationWizard(opts Options) Model {
	m := New(opts)
	m.phase = phaseMigrationWizard
	m.dockerMode = true // migration wizard always creates Docker env
	m.migrationWizard = newMigrationWizard(opts.ProjectRoot)
	return m
}

// NewApp hosts the dashboard model inside the application shell.
func NewApp(opts Options) *app.App {
	return newShell(New(opts))
}

// NewMigrationWizardApp hosts the migration-wizard model inside the shell.
func NewMigrationWizardApp(opts Options) *app.App {
	return newShell(NewMigrationWizard(opts))
}

// newShell wires a Model into the app host. Chrome and window title read the
// current model back from the shell because Model is a value type — the copy
// captured here goes stale after the first Update.
func newShell(m Model) *app.App {
	var shell *app.App
	current := func() Model {
		if cur, ok := shell.Content().(Model); ok {
			return cur
		}
		return m
	}
	shell = app.New(app.Options{
		Header:          func(ctx app.Context) string { return current().chromeHeader(ctx) },
		Footer:          func(ctx app.Context) string { return current().chromeFooter(ctx) },
		WindowTitleFunc: func(app.Context) string { return current().windowTitle() },
		// Enable mouse reporting so the overview scrolls with the wheel/trackpad
		// and the instance-log viewport reacts to the wheel.
		Mouse: true,
		// Quit handling is phase-dependent (telemetry, stop-confirm modal) and
		// stays in the model's key dispatch, so no default quit binding.
		DisableDefaultKeys: true,
	})
	m.host = shell
	shell.SetContent(m)
	return shell
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.header.Init(), m.initPhase())
}

// initPhase returns the startup command for the model's initial phase.
func (m Model) initPhase() tea.Cmd {
	if m.phase == phaseMigrationWizard {
		return nil
	}
	if m.dockerMode {
		return checkContainersRunning(m.projectRoot)
	}
	return m.checkShopwareInstalled()
}

func (m *Model) shutdown() {
	m.instance.StopStreaming()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for name, h := range m.watchers {
		if tags, ok := m.telemetry.watcherEndTags(name, watcherEndSessionEnd); ok {
			trackEventNow(tracking.EventDevWatcher, tags)
		}
		h.stop(ctx)
		delete(m.watchers, name)
	}

	// A config-change container restart may still be in flight when the user
	// leaves the dashboard; report it as cancelled instead of dropping it.
	if tags, ok := m.telemetry.configRestartTags(nil); ok {
		tags[tracking.TagResult] = tracking.ResultCancelled
		trackEventNow(tracking.EventDevDockerStart, tags)
	}

	if tags, ok := m.telemetry.sessionTags(); ok {
		trackEventNow(tracking.EventDevSession, tags)
	}
}

func (m *Model) startDashboard() tea.Cmd {
	return tea.Batch(
		m.overview.Init(),
		m.instance.StartStreaming(),
	)
}

func (m Model) Update(msg tea.Msg) (app.Content, tea.Cmd) {
	var headerCmd tea.Cmd
	m.header, headerCmd = m.header.Update(msg)

	content, cmd := m.updateContent(msg)
	return content, tea.Batch(headerCmd, cmd)
}

// updateContent routes a message by type and lifecycle phase.
func (m Model) updateContent(msg tea.Msg) (app.Content, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// The overview is scrollable, so it must know the real visible content
		// area: full height minus the dashboard's header/footer chrome and the
		// content box's border+padding (3), mirroring the app shell's layout.
		// The dashboard chrome is measured explicitly (not via the phase-aware
		// chrome*, which differs during startup phases) so the size is correct
		// even when the only WindowSizeMsg arrives before the dashboard shows.
		hdr := lipgloss.Height(buildTabHeader(m.header, int(m.activeTab), msg.Width))
		ftr := lipgloss.Height(m.renderDashboardFooter(msg.Width))
		m.overview.SetSize(m.width, max(1, msg.Height-hdr-ftr-3))
		m.instance.SetSize(m.width, m.height-4)
		m.configTab.SetSize(m.width, m.height-4)
		return m, nil

	case dockerAlreadyRunningMsg, dockerNeedStartMsg, dockerOutputLineMsg,
		dockerOutputDoneMsg, dockerStartedMsg, dockerStoppedMsg,
		shopwareInstalledMsg, shopwareNotInstalledMsg, shopwareInstallDoneMsg:
		return m.updateLifecycle(msg)

	case tui.TaskLineMsg:
		var cmd tea.Cmd
		m.task, cmd = m.task.Update(msg)
		return m, cmd

	case tui.TaskDoneMsg:
		var cmd tea.Cmd
		m.task, cmd = m.task.Update(msg)
		if tags, ok := m.telemetry.taskTags(resultTag(msg.Err)); ok {
			trackEvent(tracking.EventDevAction, tags)
		}
		return m, cmd

	case configRestartDoneMsg:
		return m.handleConfigRestartDone(msg)

	case runProxySetupRequestMsg:
		return m.runProxySetup()

	case proxySetupDoneMsg:
		// The program resumes after the interactive setup; refresh the Domains
		// status and the setup-health checks it affects.
		m.overview.domainsSetupDone = overviewSetupDone(m.projectRoot)
		m.overview.healthLoading = true
		return m, loadSetupHealth(m.projectRoot, m.executor)

	case watcherStartedMsg, watcherRunningMsg, watcherProbeMsg, stopWatcherRequestMsg,
		startStorefrontWatchRequestMsg, watcherStoppedMsg, logDoneMsg:
		return m.updateWatcherMsg(msg)

	case instancesLoadedMsg, instancesTickMsg:
		// Route to the overview directly so the self-scheduling refresh loop
		// keeps ticking even during non-dashboard phases (a running task,
		// starting/stopping); updateFallback would otherwise drop these and the
		// loop would stop for the rest of the session.
		return m.updateChildren(msg)

	case setupHealthLoadedMsg:
		if len(msg.checks) > 0 && m.telemetry.healthOnce() {
			for _, tags := range healthEventTags(msg.checks) {
				trackEvent(tracking.EventDevHealth, tags)
			}
		}
		return m.updateFallback(msg)

	case paletteResultMsg:
		return m.handlePaletteResult(msg)

	case picker.ResultMsg:
		if _, ok := msg.Key.(salesChannelPickerKey); ok {
			break
		}
		return m.applyConfigPick(msg.Key, msg.Cancelled, msg.Value)

	case textprompt.ResultMsg:
		return m.applyConfigPick(msg.Key, msg.Cancelled, msg.Value)

	case salesChannelPickerResultMsg:
		return m.handleSalesChannelPickerResult(msg)

	case prompt.ResultMsg:
		return m.handleStopConfirmResult(msg)

	case tea.KeyPressMsg:
		return m.updateKeyPress(msg)

	case tea.PasteMsg:
		return m.updatePaste(msg)
	}

	return m.updateFallback(msg)
}

// updateWatcherMsg handles the watcher lifecycle messages: start, prep
// done/failed, stop requests, stopped, and the log stream ending (the watcher
// process exiting on its own).
func (m Model) updateWatcherMsg(msg tea.Msg) (app.Content, tea.Cmd) {
	switch msg := msg.(type) {
	case watcherStartedMsg:
		m.watchers[msg.name] = msg.handle
		m.telemetry.watcherStarted(msg.name)
		return m, m.instance.AddStreamingSource(msg.name, msg.lines)

	case watcherRunningMsg:
		if msg.err != nil {
			if tags, ok := m.telemetry.watcherEndTags(msg.name, watcherEndPrepFailed); ok {
				trackEvent(tracking.EventDevWatcher, tags)
			}
		}
		_, exists := m.watchers[msg.name]
		switch msg.name {
		case watcherAdmin:
			m.overview.adminWatchStarting = false
			if msg.err == nil && exists {
				m.overview.adminWatchRunning = true
				m.overview.adminWatchReady = false
				return m, probeWatcher(watcherAdmin, m.overview.adminWatchURL)
			}
		case watcherStorefront:
			m.overview.sfWatchStarting = false
			if msg.err == nil && exists {
				m.overview.sfWatchRunning = true
				m.overview.sfWatchReady = false
				return m, probeWatcher(watcherStorefront, m.overview.sfWatchURL)
			}
		}
		return m, nil

	case watcherProbeMsg:
		// Stop probing once the watcher is gone.
		if _, exists := m.watchers[msg.name]; !exists {
			return m, nil
		}
		switch msg.name {
		case watcherAdmin:
			m.overview.adminWatchReady = msg.ready
		case watcherStorefront:
			m.overview.sfWatchReady = msg.ready
		}
		if msg.ready {
			return m, nil
		}
		// Not serving yet — keep polling until it is.
		return m, probeWatcher(msg.name, msg.url)

	case stopWatcherRequestMsg:
		return m, m.stopWatcher(msg.name)

	case startStorefrontWatchRequestMsg:
		return m.openSalesChannelPicker()

	case watcherStoppedMsg:
		switch msg.name {
		case watcherAdmin:
			m.overview.adminWatchStarting = false
			m.overview.adminWatchRunning = false
			m.overview.adminWatchReady = false
		case watcherStorefront:
			m.overview.sfWatchStarting = false
			m.overview.sfWatchRunning = false
			m.overview.sfWatchReady = false
		}
		delete(m.watchers, msg.name)
		if msg.err != nil {
			m.instance.AppendErrorLine(msg.name + " failed to start: " + msg.err.Error())
		}
		return m, nil

	case logDoneMsg:
		switch msg.source {
		case watcherAdmin:
			m.overview.adminWatchRunning = false
			m.overview.adminWatchReady = false
		case watcherStorefront:
			m.overview.sfWatchRunning = false
			m.overview.sfWatchReady = false
		}
		if tags, ok := m.telemetry.watcherEndTags(msg.source, watcherEndCrashed); ok {
			trackEvent(tracking.EventDevWatcher, tags)
		}
		delete(m.watchers, msg.source)
		return m.updateChildren(msg)
	}

	return m, nil
}

// runProxySetup runs `project proxy setup` as an interactive subprocess (it
// needs sudo), pausing the TUI via tea.ExecProcess while it runs and resuming
// afterwards.
func (m Model) runProxySetup() (app.Content, tea.Cmd) {
	bin, err := os.Executable()
	if err != nil {
		bin = "shopware-cli"
	}

	c := exec.CommandContext(context.Background(), bin, "project", "proxy", "setup")
	c.Dir = m.projectRoot

	return m, tea.ExecProcess(c, func(error) tea.Msg {
		return proxySetupDoneMsg{}
	})
}

// updateFallback handles non-key messages that aren't matched by Update's
// message-type switch, routing them by lifecycle phase.
func (m Model) updateFallback(msg tea.Msg) (app.Content, tea.Cmd) {
	if m.phase == phaseInstalling {
		switch msg := msg.(type) {
		case spinner.TickMsg:
			var cmd tea.Cmd
			m.installProg.spinner, cmd = m.installProg.spinner.Update(msg)
			return m, cmd
		case progress.FrameMsg:
			var cmd tea.Cmd
			m.installProg.progress, cmd = m.installProg.progress.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if m.phase == phaseStarting || m.phase == phaseStopping {
		if msg, ok := msg.(spinner.TickMsg); ok {
			var cmd tea.Cmd
			m.dockerSpinner, cmd = m.dockerSpinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if m.phase == phaseTask {
		var cmd tea.Cmd
		m.task, cmd = m.task.Update(msg)
		return m, cmd
	}

	if m.phase != phaseDashboard {
		return m, nil
	}

	return m.updateChildren(msg)
}

func (m Model) handlePaletteResult(msg paletteResultMsg) (app.Content, tea.Cmd) {
	if msg.ID == "" {
		return m, nil
	}
	return m.executeCommand(msg.ID)
}

// applyConfigPick handles a resolved picker or text prompt: config-tab field
// keys apply the chosen value, everything else is ignored.
func (m Model) applyConfigPick(key any, cancelled bool, value string) (app.Content, tea.Cmd) {
	if cancelled {
		return m, nil
	}
	if field, ok := key.(configField); ok {
		m.configTab.ApplyPickerValue(field, value)
	}
	return m, nil
}

func (m Model) handleSalesChannelPickerResult(msg salesChannelPickerResultMsg) (app.Content, tea.Cmd) {
	if msg.Cancelled {
		return m, nil
	}
	m.overview.sfWatchStarting = true
	return m, m.overview.startStorefrontWatch(msg.Opts)
}

func (m Model) handleStopConfirmResult(msg prompt.ResultMsg) (app.Content, tea.Cmd) {
	if msg.ID != stopConfirmID || msg.Choice == "" || msg.Choice == stopConfirmCancel {
		return m, nil
	}
	stop := msg.Choice == stopConfirmStop
	if stop {
		m.telemetry.setExitChoice(exitStopContainers)
	} else {
		m.telemetry.setExitChoice(exitKeepRunning)
	}
	m.shutdown()
	if stop {
		m.phase = phaseStopping
		m.overlayLines = nil
		m.dockerShowLogs = false
		m.dockerSpinner = tui.NewBrandSpinner()
		return m, tea.Batch(m.dockerSpinner.Tick, m.stopContainers())
	}
	return m, tea.Quit
}

func (m Model) updateChildren(msg tea.Msg) (app.Content, tea.Cmd) {
	// Key presses and mouse input must only reach the active tab, otherwise input
	// meant for one tab (e.g. Enter to pick a log source, or a wheel scroll) also
	// triggers the hidden tabs' handlers. Other messages are broadcast so
	// background updates reach every child regardless of which tab is focused.
	_, isKey := msg.(tea.KeyPressMsg)
	_, isMouse := msg.(tea.MouseMsg)
	if isKey || isMouse {
		switch m.activeTab {
		case tabOverview:
			newOverview, cmd := m.overview.Update(msg)
			m.overview = newOverview
			return m, cmd
		case tabInstance:
			newInstance, cmd := m.instance.Update(msg)
			m.instance = newInstance
			return m, cmd
		case tabConfig:
			newConfig, cmd := m.configTab.Update(msg)
			m.configTab = newConfig
			return m, cmd
		}
		return m, nil
	}

	var cmds []tea.Cmd

	newOverview, cmd := m.overview.Update(msg)
	m.overview = newOverview
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	newInstance, cmd := m.instance.Update(msg)
	m.instance = newInstance
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	newConfig, cmd := m.configTab.Update(msg)
	m.configTab = newConfig
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleConfigRestartDone(msg configRestartDoneMsg) (app.Content, tea.Cmd) {
	if tags, ok := m.telemetry.configRestartTags(msg.err); ok {
		trackEvent(tracking.EventDevDockerStart, tags)
	}
	m.configTab.restarting = false
	if msg.err != nil {
		m.configTab.err = msg.err
		m.configTab.saved = false
		return m, nil
	}

	m.configTab.saved = true
	// The restart may have changed the runtime (PHP version, published ports,
	// APP_ENV), so rediscover services and rerun the setup-health checks.
	m.overview.loading = true
	m.overview.healthLoading = true
	return m, m.overview.Init()
}

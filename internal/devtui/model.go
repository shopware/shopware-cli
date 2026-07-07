package devtui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tracking"
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
}

type Model struct {
	activeTab       activeTab
	overview        OverviewModel
	instance        InstanceModel
	configTab       ConfigModel
	width           int
	height          int
	dockerMode      bool
	phase           phase
	modal           Modal
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
	taskTitle       string
	taskDone        bool
	taskErr         error
	watchers        map[string]*watcherHandle
	migrationWizard migrationWizard
	telemetry       *telemetryState
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

type taskDoneMsg struct{ err error }

type configRestartDoneMsg struct{ err error }

func New(opts Options) Model {
	m := Model{
		activeTab:   tabOverview,
		dockerMode:  opts.Executor.Type() == executor.TypeDocker,
		projectRoot: opts.ProjectRoot,
		executor:    opts.Executor,
		config:      opts.Config,
		envConfig:   opts.EnvConfig,
		watchers:    make(map[string]*watcherHandle),
		telemetry:   newTelemetryState(opts.Executor.Type() == executor.TypeDocker),
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

func (m Model) Init() tea.Cmd {
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

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.overview.SetSize(m.width, m.height-4)
		m.instance.SetSize(m.width, m.height-4)
		m.configTab.SetSize(m.width, m.height-4)
		return m, nil

	case dockerAlreadyRunningMsg, dockerNeedStartMsg, dockerOutputLineMsg,
		dockerOutputDoneMsg, dockerStartedMsg, dockerStoppedMsg,
		shopwareInstalledMsg, shopwareNotInstalledMsg, shopwareInstallDoneMsg:
		return m.updateLifecycle(msg)

	case taskDoneMsg:
		m.taskDone = true
		m.taskErr = msg.err
		if tags, ok := m.telemetry.taskTags(resultTag(msg.err)); ok {
			trackEvent(tracking.EventDevAction, tags)
		}
		if msg.err != nil {
			m.overlayLines = append(m.overlayLines, "", errorStyle.Render("Failed: "+msg.err.Error()))
		} else {
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Done. Press any key to close."))
		}
		return m, nil

	case configRestartDoneMsg:
		return m.handleConfigRestartDone(msg)

	case watcherStartedMsg, watcherRunningMsg, stopWatcherRequestMsg,
		startStorefrontWatchRequestMsg, watcherStoppedMsg, logDoneMsg:
		return m.updateWatcherMsg(msg)

	case setupHealthLoadedMsg:
		if len(msg.checks) > 0 && m.telemetry.healthOnce() {
			for _, tags := range healthEventTags(msg.checks) {
				trackEvent(tracking.EventDevHealth, tags)
			}
		}
		return m.updateFallback(msg)

	case paletteResultMsg:
		return m.handlePaletteResult(msg)

	case pickerResultMsg:
		if _, ok := msg.Key.(salesChannelPickerKey); ok {
			break
		}
		return m.handlePickerResult(msg)

	case salesChannelPickerResultMsg:
		return m.handleSalesChannelPickerResult(msg)

	case stopConfirmResultMsg:
		return m.handleStopConfirmResult(msg)

	case tea.KeyPressMsg:
		return m.updateKeyPress(msg)
	}

	return m.updateFallback(msg)
}

// updateWatcherMsg handles the watcher lifecycle messages: start, prep
// done/failed, stop requests, stopped, and the log stream ending (the watcher
// process exiting on its own).
func (m Model) updateWatcherMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			}
		case watcherStorefront:
			m.overview.sfWatchStarting = false
			if msg.err == nil && exists {
				m.overview.sfWatchRunning = true
			}
		}
		return m, nil

	case stopWatcherRequestMsg:
		return m, m.stopWatcher(msg.name)

	case startStorefrontWatchRequestMsg:
		return m.openSalesChannelPicker()

	case watcherStoppedMsg:
		switch msg.name {
		case watcherAdmin:
			m.overview.adminWatchStarting = false
			m.overview.adminWatchRunning = false
		case watcherStorefront:
			m.overview.sfWatchStarting = false
			m.overview.sfWatchRunning = false
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
		case watcherStorefront:
			m.overview.sfWatchRunning = false
		}
		if tags, ok := m.telemetry.watcherEndTags(msg.source, watcherEndCrashed); ok {
			trackEvent(tracking.EventDevWatcher, tags)
		}
		delete(m.watchers, msg.source)
		return m.updateChildren(msg)
	}

	return m, nil
}

// updateFallback handles non-key messages that aren't matched by Update's
// message-type switch, routing them by modal state and lifecycle phase.
func (m Model) updateFallback(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.modal != nil {
		next, cmd := m.modal.Update(msg)
		m.modal = next
		return m, cmd
	}

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
		// Keep ticking the header spinner while the task runs; stop once it's
		// done so the final output stays static.
		if msg, ok := msg.(spinner.TickMsg); ok {
			if m.taskDone {
				return m, nil
			}
			var cmd tea.Cmd
			m.dockerSpinner, cmd = m.dockerSpinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if m.phase != phaseDashboard {
		return m, nil
	}

	return m.updateChildren(msg)
}

func (m Model) handlePaletteResult(msg paletteResultMsg) (tea.Model, tea.Cmd) {
	m.modal = nil
	if msg.ID == "" {
		return m, nil
	}
	return m.executeCommand(msg.ID)
}

func (m Model) handlePickerResult(msg pickerResultMsg) (tea.Model, tea.Cmd) {
	m.modal = nil
	if msg.Cancelled {
		return m, nil
	}
	if field, ok := msg.Key.(configField); ok {
		m.configTab.ApplyPickerValue(field, msg.Value)
	}
	return m, nil
}

func (m Model) handleSalesChannelPickerResult(msg salesChannelPickerResultMsg) (tea.Model, tea.Cmd) {
	m.modal = nil
	if msg.Cancelled {
		return m, nil
	}
	m.overview.sfWatchStarting = true
	return m, m.overview.startStorefrontWatch(msg.Opts)
}

func (m Model) handleStopConfirmResult(msg stopConfirmResultMsg) (tea.Model, tea.Cmd) {
	m.modal = nil
	if msg.Cancel {
		return m, nil
	}
	if msg.Stop {
		m.telemetry.setExitChoice(exitStopContainers)
	} else {
		m.telemetry.setExitChoice(exitKeepRunning)
	}
	m.shutdown()
	if msg.Stop {
		m.phase = phaseStopping
		m.overlayLines = nil
		m.dockerShowLogs = false
		m.dockerSpinner = newBrandSpinner()
		return m, tea.Batch(m.dockerSpinner.Tick, m.stopContainers())
	}
	return m, tea.Quit
}

func (m Model) updateChildren(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Key presses must only reach the active tab, otherwise a key meant for one
	// tab (e.g. Enter to pick a log source) also triggers the hidden tabs'
	// handlers. Non-key messages are broadcast so background updates reach every
	// child regardless of which tab is focused.
	if _, isKey := msg.(tea.KeyPressMsg); isKey {
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

func (m Model) handleConfigRestartDone(msg configRestartDoneMsg) (tea.Model, tea.Cmd) {
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

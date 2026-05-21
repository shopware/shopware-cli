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
)

type activeTab int

const (
	tabGeneral activeTab = iota
	tabLogs
	tabConfig
)

var tabNames = []string{"General", "Logs", "Config"}

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
	phaseSetupGuide
)

type Options struct {
	ProjectRoot string
	Config      *shop.Config
	EnvConfig   *shop.EnvironmentConfig
	Executor    executor.Executor
}

type Model struct {
	activeTab      activeTab
	general        GeneralModel
	logs           LogsModel
	configTab      ConfigModel
	width          int
	height         int
	dockerMode     bool
	phase          phase
	modal          Modal
	overlayLines   []string
	projectRoot    string
	executor       executor.Executor
	dockerOutChan  <-chan string
	install        installWizard
	installProg    installProgress
	dockerSpinner  spinner.Model
	dockerShowLogs bool
	config         *shop.Config
	envConfig      *shop.EnvironmentConfig
	taskTitle      string
	taskDone       bool
	taskErr        error
	watchers       map[string]*executor.Process
	setupGuide     setupGuide
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

func New(opts Options) Model {
	effectiveAdminApi := opts.Config.AdminApi
	if opts.EnvConfig.AdminApi != nil {
		effectiveAdminApi = opts.EnvConfig.AdminApi
	}

	shopURL := opts.Config.URL
	if opts.EnvConfig.URL != "" {
		shopURL = opts.EnvConfig.URL
	}

	var username, password string
	if effectiveAdminApi != nil {
		username = effectiveAdminApi.Username
		password = effectiveAdminApi.Password
	}

	isDocker := opts.Executor.Type() == executor.TypeDocker

	envValues, _ := envfile.ReadValues(opts.ProjectRoot, EnvFieldKeys()...)

	return Model{
		activeTab:   tabGeneral,
		general:     NewGeneralModel(opts.Executor.Type(), shopURL, username, password, opts.ProjectRoot, opts.Executor, opts.Config),
		logs:        NewLogsModel(opts.ProjectRoot, isDocker),
		configTab:   NewConfigModel(opts.Config, envValues),
		dockerMode:  isDocker,
		projectRoot: opts.ProjectRoot,
		executor:    opts.Executor,
		config:      opts.Config,
		envConfig:   opts.EnvConfig,
		watchers:    make(map[string]*executor.Process),
	}
}

// NewSetupGuide creates a Model that starts in the setup guide phase
// for projects that don't yet have a development environment configured.
func NewSetupGuide(opts Options) Model {
	m := New(opts)
	m.phase = phaseSetupGuide
	m.dockerMode = true // setup guide always creates Docker env
	m.setupGuide = newSetupGuide(opts.ProjectRoot)
	return m
}

func (m Model) Init() tea.Cmd {
	if m.phase == phaseSetupGuide {
		return nil
	}
	if m.dockerMode {
		return checkContainersRunning(m.projectRoot)
	}
	return m.checkShopwareInstalled()
}

func (m *Model) shutdown() {
	m.logs.StopStreaming()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	for name, p := range m.watchers {
		_ = p.Stop(ctx)
		delete(m.watchers, name)
	}
}

func (m *Model) startDashboard() tea.Cmd {
	return tea.Batch(
		m.general.Init(),
		m.logs.StartStreaming(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.general.SetSize(m.width, m.height-4)
		m.logs.SetSize(m.width, m.height-4)
		m.configTab.SetSize(m.width, m.height-4)
		return m, nil

	case dockerAlreadyRunningMsg, dockerNeedStartMsg, dockerOutputLineMsg,
		dockerOutputDoneMsg, dockerStartedMsg, dockerStoppedMsg,
		shopwareInstalledMsg, shopwareNotInstalledMsg, shopwareInstallDoneMsg:
		return m.updateLifecycle(msg)

	case taskDoneMsg:
		m.taskDone = true
		m.taskErr = msg.err
		if msg.err != nil {
			m.overlayLines = append(m.overlayLines, "", errorStyle.Render("Failed: "+msg.err.Error()))
		} else {
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Done. Press any key to close."))
		}
		return m, nil

	case watcherStartedMsg:
		switch msg.name {
		case watcherAdmin:
			m.general.adminWatchStarting = false
			m.general.adminWatchRunning = true
		case watcherStorefront:
			m.general.sfWatchStarting = false
			m.general.sfWatchRunning = true
		}
		m.watchers[msg.name] = msg.process
		m.activeTab = tabLogs
		return m, m.logs.AddProcessSource(msg.name, msg.process)

	case watcherStoppedMsg:
		switch msg.name {
		case watcherAdmin:
			m.general.adminWatchStarting = false
			m.general.adminWatchRunning = false
		case watcherStorefront:
			m.general.sfWatchStarting = false
			m.general.sfWatchRunning = false
		}
		delete(m.watchers, msg.name)
		if msg.err != nil {
			m.logs.AppendErrorLine(msg.name + " failed to start: " + msg.err.Error())
			m.activeTab = tabLogs
		}
		return m, nil

	case logDoneMsg:
		name := m.logs.ActiveProcessSourceName()
		switch name {
		case watcherAdmin:
			m.general.adminWatchRunning = false
		case watcherStorefront:
			m.general.sfWatchRunning = false
		}
		delete(m.watchers, name)
		return m.updateChildren(msg)

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
	m.general.sfWatchStarting = true
	return m, m.general.startStorefrontWatch(msg.Opts)
}

func (m Model) handleStopConfirmResult(msg stopConfirmResultMsg) (tea.Model, tea.Cmd) {
	m.modal = nil
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
	var cmds []tea.Cmd

	newGeneral, cmd := m.general.Update(msg)
	m.general = newGeneral
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	newLogs, cmd := m.logs.Update(msg)
	m.logs = newLogs
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

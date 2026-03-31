package devtui

import (
	"context"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

type activeTab int

const (
	tabGeneral activeTab = iota
	tabLogs
)

var tabNames = []string{"General", "Logs"}

const (
	keyCtrlC    = "ctrl+c"
	keyDown     = "down"
	keyEnter    = "enter"
	keyUp       = "up"
	keyTab      = "tab"
	keyShiftTab = "shift+tab"
	keyQ        = "q"
	keyF        = "f"
	keyJ        = "j"
	keyK        = "k"
	key1        = "1"
	key2        = "2"

	defaultUsername = "admin"

	watcherAdmin      = "Admin Watcher"
	watcherStorefront = "Storefront Watcher"
)

type overlay int

const (
	overlayNone overlay = iota
	overlayStarting
	overlayStopConfirm
	overlayStopping
	overlayInstallPrompt
	overlayInstalling
	overlayCommandPalette
	overlayTask
)

type installStep int

const (
	installStepAsk installStep = iota
	installStepLanguage
	installStepCurrency
	installStepUsername
	installStepPassword
)

type installLanguage struct {
	id    string
	label string
}

var (
	installLanguages = []installLanguage{
		{"en-GB", "English (UK)"},
		{"en-US", "English (US)"},
		{"de-DE", "Deutsch"},
		{"cs-CZ", "Čeština"},
		{"da-DK", "Dansk"},
		{"es-ES", "Español"},
		{"fr-FR", "Français"},
		{"it-IT", "Italiano"},
		{"nl-NL", "Nederlands"},
		{"nn-NO", "Norsk"},
		{"pl-PL", "Język polski"},
		{"pt-PT", "Português"},
		{"sv-SE", "Svenska"},
	}
	installCurrencies = []string{"EUR", "USD", "GBP", "PLN", "CHF", "SEK", "DKK", "NOK", "CZK"}
)

type installWizard struct {
	step       installStep
	cursor     int
	confirmYes bool
	language   string
	currency   string
	username   textinput.Model
	password   textinput.Model
}

type Options struct {
	ProjectRoot string
	Config      *shop.Config
	EnvConfig   *shop.EnvironmentConfig
	Executor    executor.Executor
}

type installProgress struct {
	currentStep int
	done        bool
	showLogs    bool
	spinner     spinner.Model
	progress    progress.Model
}

var installStepPatterns = []struct {
	pattern string
	label   string
}{
	{"system:install", "Installing Shopware"},
	{"user:create", "Creating admin account"},
	{"messenger:setup-transports", "Setting up message transports"},
	{"sales-channel:create:storefront", "Creating storefront"},
	{"theme:change", "Compiling theme"},
	{"plugin:refresh", "Refreshing plugins"},
}

type Model struct {
	activeTab      activeTab
	general        GeneralModel
	logs           LogsModel
	width          int
	height         int
	dockerMode     bool
	overlay        overlay
	overlayLines   []string
	projectRoot    string
	executor       executor.Executor
	dockerOutChan  <-chan string
	install        installWizard
	installProg    installProgress
	stopConfirmYes bool
	dockerSpinner  spinner.Model
	dockerShowLogs bool
	palette        commandPalette
	config         *shop.Config
	envConfig      *shop.EnvironmentConfig
	taskTitle      string
	taskDone       bool
	taskErr        error
	watchers       map[string]*executor.Process // running watcher processes keyed by name
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

	isDocker := opts.Executor.Type() == "docker"

	return Model{
		activeTab:   tabGeneral,
		general:     NewGeneralModel(opts.Executor.Type(), shopURL, username, password, opts.ProjectRoot, opts.Executor),
		logs:        NewLogsModel(opts.ProjectRoot, isDocker),
		dockerMode:  isDocker,
		projectRoot: opts.ProjectRoot,
		executor:    opts.Executor,
		config:      opts.Config,
		envConfig:   opts.EnvConfig,
		watchers:    make(map[string]*executor.Process),
	}
}

func (m Model) Init() tea.Cmd {
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

	case tea.KeyPressMsg:
		return m.updateKeyPress(msg)
	}

	if m.overlay == overlayInstalling {
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

	if m.overlay == overlayStarting || m.overlay == overlayStopping {
		if msg, ok := msg.(spinner.TickMsg); ok {
			var cmd tea.Cmd
			m.dockerSpinner, cmd = m.dockerSpinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	if m.overlay != overlayNone {
		return m, nil
	}

	return m.updateChildren(msg)
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

	return m, tea.Batch(cmds...)
}

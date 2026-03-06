package devtui

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

type activeTab int

const (
	tabGeneral activeTab = iota
	tabLogs
)

var tabNames = []string{"General", "Logs"}

// Key constants for repeated key strings
const (
	keyCtrlC    = "ctrl+c"
	keyDown     = "down"
	keyEnter    = "enter"
	keyUp       = "up"
	keyTab      = "tab"
	keyShiftTab = "shift+tab"
	keyQ        = "q"
	keyY        = "y"
	keyYUpper   = "Y"
	keyN        = "n"
	keyNUpper   = "N"
	keyF        = "f"
	keyA        = "a"
	keyJ        = "j"
	keyK        = "k"
	key1        = "1"
	key2        = "2"
)

type overlay int

const (
	overlayNone overlay = iota
	overlayStarting
	overlayStopConfirm
	overlayStopping
	overlayInstallPrompt
	overlayInstalling
)

// installStep tracks which question the install wizard is on.
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

// installWizard holds state for the Shopware install prompt.
type installWizard struct {
	step     installStep
	cursor   int
	language string
	currency string
	username textinput.Model
	password textinput.Model
}

// Options configures the TUI dashboard.
type Options struct {
	ProjectRoot string
	Config      *shop.Config
	EnvConfig   *shop.EnvironmentConfig
	Executor    executor.Executor
}

// Model is the top-level Bubble Tea model for the dev dashboard.
type Model struct {
	activeTab     activeTab
	general       GeneralModel
	logs          LogsModel
	width         int
	height        int
	dockerMode    bool
	overlay       overlay
	overlayLines  []string
	projectRoot   string
	executor      executor.Executor
	dockerOutChan <-chan string
	install       installWizard
	config        *shop.Config
	envConfig     *shop.EnvironmentConfig
}

// docker lifecycle messages
type dockerAlreadyRunningMsg struct{}
type dockerNeedStartMsg struct{}
type dockerStartedMsg struct{ err error }
type dockerStoppedMsg struct{ err error }
type dockerOutputLineMsg string
type dockerOutputDoneMsg struct{}

// shopware install check messages
type shopwareInstalledMsg struct{}
type shopwareNotInstalledMsg struct{}
type shopwareInstallDoneMsg struct{ err error }

// New creates a new TUI model from the given options.
func New(opts Options) Model {
	// Resolve effective admin API: environment overrides top-level config
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
		general:     NewGeneralModel(opts.Executor.Type(), shopURL, username, password, opts.ProjectRoot),
		logs:        NewLogsModel(opts.ProjectRoot, isDocker),
		dockerMode:  isDocker,
		projectRoot: opts.ProjectRoot,
		executor:    opts.Executor,
		config:      opts.Config,
		envConfig:   opts.EnvConfig,
	}
}

func (m Model) Init() tea.Cmd {
	if m.dockerMode {
		return checkContainersRunning(m.projectRoot)
	}
	return m.checkShopwareInstalled()
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
		m.logs.SetSize(m.width, m.height-4)
		return m, nil

	case dockerAlreadyRunningMsg, dockerNeedStartMsg, dockerOutputLineMsg,
		dockerOutputDoneMsg, dockerStartedMsg, dockerStoppedMsg,
		shopwareInstalledMsg, shopwareNotInstalledMsg, shopwareInstallDoneMsg:
		return m.updateLifecycle(msg)

	case tea.KeyPressMsg:
		return m.updateKeyPress(msg)
	}

	if m.overlay != overlayNone {
		return m, nil
	}

	return m.updateChildren(msg)
}

// updateLifecycle handles docker and shopware lifecycle messages.
func (m Model) updateLifecycle(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dockerAlreadyRunningMsg:
		m.overlay = overlayNone
		return m, m.checkShopwareInstalled()

	case dockerNeedStartMsg:
		m.overlay = overlayStarting
		m.overlayLines = nil
		return m, m.startContainers()

	case dockerOutputLineMsg:
		m.overlayLines = append(m.overlayLines, string(msg))
		if len(m.overlayLines) > 10 {
			m.overlayLines = m.overlayLines[len(m.overlayLines)-10:]
		}
		return m, m.readNextDockerOutput()

	case dockerOutputDoneMsg:
		return m, nil

	case dockerStartedMsg:
		if msg.err != nil {
			m.overlayLines = append(m.overlayLines, errorStyle.Render("Failed: "+msg.err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}
		m.overlay = overlayNone
		m.overlayLines = nil
		m.dockerOutChan = nil
		return m, m.checkShopwareInstalled()

	case shopwareInstalledMsg:
		m.overlay = overlayNone
		return m, m.startDashboard()

	case shopwareNotInstalledMsg:
		m.overlay = overlayInstallPrompt
		m.overlayLines = nil

		usernameInput := textinput.New()
		usernameInput.Placeholder = "admin"
		usernameInput.Prompt = "Username: "
		usernameInput.CharLimit = 50

		passwordInput := textinput.New()
		passwordInput.Placeholder = "shopware"
		passwordInput.Prompt = "Password: "
		passwordInput.CharLimit = 50

		m.install = installWizard{step: installStepAsk, username: usernameInput, password: passwordInput}
		return m, nil

	case shopwareInstallDoneMsg:
		if msg.err != nil {
			m.overlayLines = append(m.overlayLines, "", errorStyle.Render("Installation failed: "+msg.err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}

		// Update config with admin credentials
		username := m.install.username.Value()
		password := m.install.password.Value()

		adminApi := &shop.ConfigAdminApi{
			Username: username,
			Password: password,
		}
		m.envConfig.AdminApi = adminApi
		_ = shop.WriteConfig(m.config, m.projectRoot)

		// Update general tab display
		m.general.username = username
		m.general.password = password

		m.overlay = overlayNone
		m.overlayLines = nil
		m.dockerOutChan = nil
		return m, m.startDashboard()

	case dockerStoppedMsg:
		return m, tea.Quit
	}

	return m, nil
}

// updateKeyPress handles all key press events, including overlay-specific keys.
func (m Model) updateKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.overlay == overlayInstallPrompt {
		return m.updateInstallPrompt(msg)
	}

	if m.overlay == overlayStopConfirm {
		switch msg.String() {
		case keyY, keyYUpper:
			m.overlay = overlayStopping
			m.overlayLines = nil
			return m, m.stopContainers()
		case keyN, keyNUpper:
			return m, tea.Quit
		}
		return m, nil
	}

	// Other overlays only allow quit
	if m.overlay != overlayNone {
		if msg.String() == keyQ || msg.String() == keyCtrlC {
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg.String() {
	case keyCtrlC, keyQ:
		m.logs.StopStreaming()
		if m.dockerMode {
			m.overlay = overlayStopConfirm
			m.overlayLines = nil
			return m, nil
		}
		return m, tea.Quit
	case key1:
		m.activeTab = tabGeneral
		return m, nil
	case key2:
		m.activeTab = tabLogs
		return m, nil
	case keyTab, keyShiftTab:
		m.activeTab = (m.activeTab + 1) % 2
		return m, nil
	case keyF:
		if m.activeTab == tabGeneral {
			return m, openInBrowser(m.general.shopURL)
		}
	case keyA:
		if m.activeTab == tabGeneral {
			return m, openInBrowser(m.general.adminURL)
		}
	}

	return m.updateChildren(msg)
}

// updateChildren propagates messages to child models.
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

// updateInstallPrompt handles key input for the install wizard steps.
func (m Model) updateInstallPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyQ, keyCtrlC:
		return m, tea.Quit
	}

	switch m.install.step {
	case installStepAsk:
		switch msg.String() {
		case keyY, keyYUpper:
			m.install.step = installStepLanguage
			m.install.cursor = 0
		case keyN, keyNUpper:
			m.overlay = overlayNone
			return m, m.startDashboard()
		}

	case installStepLanguage:
		switch msg.String() {
		case keyUp, keyK:
			if m.install.cursor > 0 {
				m.install.cursor--
			}
		case keyDown, keyJ:
			if m.install.cursor < len(installLanguages)-1 {
				m.install.cursor++
			}
		case keyEnter:
			m.install.language = installLanguages[m.install.cursor].id
			m.install.step = installStepCurrency
			m.install.cursor = 0
		}

	case installStepCurrency:
		switch msg.String() {
		case keyUp, keyK:
			if m.install.cursor > 0 {
				m.install.cursor--
			}
		case keyDown, keyJ:
			if m.install.cursor < len(installCurrencies)-1 {
				m.install.cursor++
			}
		case keyEnter:
			m.install.currency = installCurrencies[m.install.cursor]
			m.install.step = installStepUsername
			m.install.username.SetValue("admin")
			m.install.username.Focus()
			return m, textinput.Blink
		}

	case installStepUsername:
		switch msg.String() {
		case keyEnter:
			m.install.step = installStepPassword
			m.install.username.Blur()
			m.install.password.SetValue("shopware")
			m.install.password.Focus()
			return m, textinput.Blink
		default:
			var cmd tea.Cmd
			m.install.username, cmd = m.install.username.Update(msg)
			return m, cmd
		}

	case installStepPassword:
		switch msg.String() {
		case keyEnter:
			m.install.password.Blur()
			m.overlay = overlayInstalling
			m.overlayLines = nil
			return m, m.runShopwareInstall()
		default:
			var cmd tea.Cmd
			m.install.password, cmd = m.install.password.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	var b strings.Builder

	if m.overlay != overlayNone {
		b.WriteString(m.renderOverlay())
	} else {
		b.WriteString(m.renderTabBar())
		b.WriteString("\n")

		switch m.activeTab {
		case tabGeneral:
			b.WriteString(m.general.View())
		case tabLogs:
			b.WriteString(m.logs.View())
		}
	}

	v := tea.NewView(b.String())
	v.AltScreen = true
	return v
}

func (m Model) renderOverlay() string {
	var title string
	switch m.overlay {
	case overlayNone:
		title = ""
	case overlayStarting:
		title = "Starting Docker containers..."
	case overlayStopConfirm:
		title = "Stop Docker containers?"
	case overlayStopping:
		title = "Stopping Docker containers..."
	case overlayInstallPrompt:
		title = "Shopware is not installed"
	case overlayInstalling:
		title = "Installing Shopware..."
	}

	var content strings.Builder
	content.WriteString(statusStyle.Render(title))
	content.WriteString("\n\n")

	switch m.overlay {
	case overlayNone:
		// No overlay content needed
	case overlayStarting, overlayStopping, overlayInstalling:
		for _, line := range m.overlayLines {
			content.WriteString(line + "\n")
		}
	case overlayStopConfirm:
		content.WriteString("Do you want to stop the Docker containers?\n\n")
		content.WriteString(helpStyle.Render("y: stop containers | n: quit without stopping"))
	case overlayInstallPrompt:
		m.renderInstallPrompt(&content)
	}

	modal := overlayStyle.Render(content.String())

	if m.width > 0 && m.height > 0 {
		modal = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	}

	return modal
}

func (m Model) renderInstallPrompt(b *strings.Builder) {
	switch m.install.step {
	case installStepAsk:
		b.WriteString("Would you like to install Shopware now?\n\n")
		b.WriteString(helpStyle.Render("y: install | n: skip | q: quit"))

	case installStepLanguage:
		b.WriteString("Select default language:\n\n")
		for i, lang := range installLanguages {
			cursor := "  "
			if i == m.install.cursor {
				cursor = "> "
			}
			b.WriteString(cursor + lang.label + "\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("↑/↓: select | enter: confirm | q: quit"))

	case installStepCurrency:
		fmt.Fprintf(b, "Language: %s\n\n", valueStyle.Render(m.install.language))
		b.WriteString("Select default currency:\n\n")
		for i, curr := range installCurrencies {
			cursor := "  "
			if i == m.install.cursor {
				cursor = "> "
			}
			b.WriteString(cursor + curr + "\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("↑/↓: select | enter: confirm | q: quit"))

	case installStepUsername:
		fmt.Fprintf(b, "Language: %s\n", valueStyle.Render(m.install.language))
		fmt.Fprintf(b, "Currency: %s\n\n", valueStyle.Render(m.install.currency))
		b.WriteString("Admin username:\n\n")
		b.WriteString(m.install.username.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter: confirm | q: quit"))

	case installStepPassword:
		fmt.Fprintf(b, "Language: %s\n", valueStyle.Render(m.install.language))
		fmt.Fprintf(b, "Currency: %s\n", valueStyle.Render(m.install.currency))
		fmt.Fprintf(b, "Username: %s\n\n", valueStyle.Render(m.install.username.Value()))
		b.WriteString("Admin password:\n\n")
		b.WriteString(m.install.password.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("enter: confirm | q: quit"))
	}
}

func (m Model) renderTabBar() string {
	var tabs []string
	for i, name := range tabNames {
		label := fmt.Sprintf(" %d: %s ", i+1, name)
		if activeTab(i) == m.activeTab {
			tabs = append(tabs, activeTabStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

// checkContainersRunning checks if any containers are already running.
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

// checkShopwareInstalled runs bin/console system:is-installed to check if Shopware is set up.
func (m *Model) checkShopwareInstalled() tea.Cmd {
	exec := m.executor
	projectRoot := m.projectRoot
	return func() tea.Msg {
		cmd := exec.ConsoleCommand(context.Background(), "system:is-installed")
		cmd.Dir = projectRoot
		if err := cmd.Run(); err != nil {
			return shopwareNotInstalledMsg{}
		}
		return shopwareInstalledMsg{}
	}
}

// runShopwareInstall runs vendor/bin/shopware-deployment-helper run with INSTALL_LOCALE and INSTALL_CURRENCY env vars.
func (m *Model) runShopwareInstall() tea.Cmd {
	e := m.executor
	projectRoot := m.projectRoot
	language := m.install.language
	currency := m.install.currency

	ch := make(chan string, 50)
	m.dockerOutChan = ch

	outputCmd := func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return dockerOutputDoneMsg{}
		}
		return dockerOutputLineMsg(line)
	}

	username := m.install.username.Value()
	password := m.install.password.Value()

	doneCmd := func() tea.Msg {
		ctx := executor.WithEnv(context.Background(), map[string]string{
			"INSTALL_LOCALE":         language,
			"INSTALL_CURRENCY":       currency,
			"INSTALL_ADMIN_USERNAME": username,
			"INSTALL_ADMIN_PASSWORD": password,
		})
		cmd := e.PHPCommand(ctx, "vendor/bin/shopware-deployment-helper", "run")
		cmd.Dir = projectRoot

		pipe, err := cmd.StdoutPipe()
		if err != nil {
			close(ch)
			return shopwareInstallDoneMsg{err: err}
		}
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			close(ch)
			return shopwareInstallDoneMsg{err: err}
		}

		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			ch <- scanner.Text()
		}
		close(ch)

		err = cmd.Wait()
		return shopwareInstallDoneMsg{err: err}
	}

	return tea.Batch(outputCmd, doneCmd)
}

// readNextDockerOutput reads the next line from the docker output channel.
func (m *Model) readNextDockerOutput() tea.Cmd {
	ch := m.dockerOutChan
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return dockerOutputDoneMsg{}
		}
		return dockerOutputLineMsg(line)
	}
}

// runDockerCommandWithArgs runs a docker compose command, streaming stderr lines
// through a channel for display, and returns a result message when done.
func runDockerCommandWithArgs(ctx context.Context, projectRoot string, args []string, resultFn func(error) tea.Msg) (outChan <-chan string, outputCmd tea.Cmd, doneCmd tea.Cmd) {
	lineChan := make(chan string, 50)

	outputCmd = func() tea.Msg {
		line, ok := <-lineChan
		if !ok {
			return dockerOutputDoneMsg{}
		}
		return dockerOutputLineMsg(line)
	}

	doneCmd = func() tea.Msg {
		cmd := exec.CommandContext(ctx, "docker", args...)
		cmd.Dir = projectRoot

		pipe, err := cmd.StderrPipe()
		if err != nil {
			close(lineChan)
			return resultFn(err)
		}
		cmd.Stdout = cmd.Stderr

		if err := cmd.Start(); err != nil {
			close(lineChan)
			return resultFn(err)
		}

		scanner := bufio.NewScanner(pipe)
		for scanner.Scan() {
			lineChan <- scanner.Text()
		}
		close(lineChan)

		err = cmd.Wait()
		return resultFn(err)
	}

	return lineChan, outputCmd, doneCmd
}

// startContainers runs docker compose up -d, streaming output.
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

// stopContainers runs docker compose down, streaming output.
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

package devtui

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
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

	defaultUsername = "admin"
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
		m.general.SetSize(m.width, m.height-4)
		m.logs.SetSize(m.width, m.height-4)
		return m, nil

	case dockerAlreadyRunningMsg, dockerNeedStartMsg, dockerOutputLineMsg,
		dockerOutputDoneMsg, dockerStartedMsg, dockerStoppedMsg,
		shopwareInstalledMsg, shopwareNotInstalledMsg, shopwareInstallDoneMsg:
		return m.updateLifecycle(msg)

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

func (m Model) updateLifecycle(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case dockerAlreadyRunningMsg:
		m.overlay = overlayNone
		return m, m.checkShopwareInstalled()

	case dockerNeedStartMsg:
		m.overlay = overlayStarting
		m.overlayLines = nil
		m.dockerShowLogs = false
		m.dockerSpinner = spinner.New(
			spinner.WithSpinner(spinner.Dot),
			spinner.WithStyle(lipgloss.NewStyle().Foreground(tui.BrandColor)),
		)
		return m, tea.Batch(m.dockerSpinner.Tick, m.startContainers())

	case dockerOutputLineMsg:
		m.overlayLines = append(m.overlayLines, string(msg))
		maxLines := m.overlayMaxLines()
		if len(m.overlayLines) > maxLines {
			m.overlayLines = m.overlayLines[len(m.overlayLines)-maxLines:]
		}
		if m.overlay == overlayInstalling {
			line := string(msg)
			if strings.HasPrefix(line, "Start: ") {
				for i, sp := range installStepPatterns {
					if strings.Contains(line, sp.pattern) && i >= m.installProg.currentStep {
						m.installProg.currentStep = i
						pct := float64(i) / float64(len(installStepPatterns))
						cmd := m.installProg.progress.SetPercent(pct)
						return m, tea.Batch(cmd, m.readNextDockerOutput())
					}
				}
			}
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
		usernameInput.Placeholder = defaultUsername
		usernameInput.Prompt = "Username: "
		usernameInput.CharLimit = 50

		passwordInput := textinput.New()
		passwordInput.Placeholder = "shopware"
		passwordInput.Prompt = "Password: "
		passwordInput.CharLimit = 50

		m.install = installWizard{step: installStepAsk, confirmYes: true, username: usernameInput, password: passwordInput}
		return m, nil

	case shopwareInstallDoneMsg:
		if msg.err != nil {
			m.installProg.showLogs = true
			m.overlayLines = append(m.overlayLines, "", errorStyle.Render("Installation failed: "+msg.err.Error()))
			m.overlayLines = append(m.overlayLines, "", helpStyle.Render("Press q to exit"))
			return m, nil
		}
		m.installProg.done = true
		m.installProg.currentStep = len(installStepPatterns)

		username := m.install.username.Value()
		password := m.install.password.Value()

		adminApi := &shop.ConfigAdminApi{
			Username: username,
			Password: password,
		}
		m.envConfig.AdminApi = adminApi
		_ = shop.WriteConfig(m.config, m.projectRoot)

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

func (m Model) updateKeyPress(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.overlay == overlayCommandPalette {
		return m.updateCommandPalette(msg)
	}

	if m.overlay == overlayInstallPrompt {
		return m.updateInstallPrompt(msg)
	}

	if m.overlay == overlayStopConfirm {
		switch msg.String() {
		case "left", "h":
			m.stopConfirmYes = true
		case "right", "l":
			m.stopConfirmYes = false
		case keyTab:
			m.stopConfirmYes = !m.stopConfirmYes
		case keyEnter:
			if m.stopConfirmYes {
				m.overlay = overlayStopping
				m.overlayLines = nil
				m.dockerShowLogs = false
				m.dockerSpinner = spinner.New(
					spinner.WithSpinner(spinner.Dot),
					spinner.WithStyle(lipgloss.NewStyle().Foreground(tui.BrandColor)),
				)
				return m, tea.Batch(m.dockerSpinner.Tick, m.stopContainers())
			}
			return m, tea.Quit
		}
		return m, nil
	}

	if m.overlay == overlayStarting || m.overlay == overlayStopping {
		switch msg.String() {
		case "l":
			m.dockerShowLogs = !m.dockerShowLogs
		case keyQ, keyCtrlC:
			return m, tea.Quit
		}
		return m, nil
	}

	if m.overlay == overlayInstalling {
		switch msg.String() {
		case "l":
			m.installProg.showLogs = !m.installProg.showLogs
		case keyQ, keyCtrlC:
			return m, tea.Quit
		}
		return m, nil
	}

	if m.overlay != overlayNone {
		if msg.String() == keyQ || msg.String() == keyCtrlC {
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg.String() {
	case "ctrl+p":
		m.overlay = overlayCommandPalette
		m.palette = newCommandPalette()
		return m, textinput.Blink
	case keyCtrlC, keyQ:
		m.logs.StopStreaming()
		if m.dockerMode {
			m.overlay = overlayStopConfirm
			m.overlayLines = nil
			m.stopConfirmYes = true
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
	}

	return m.updateChildren(msg)
}

func (m Model) updateCommandPalette(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+p":
		m.overlay = overlayNone
		return m, nil
	case keyUp, keyK:
		m.palette.moveUp()
		return m, nil
	case keyDown, keyJ:
		m.palette.moveDown()
		return m, nil
	case keyEnter:
		id := m.palette.selectedID()
		m.overlay = overlayNone
		return m.executeCommand(id)
	}

	var cmd tea.Cmd
	m.palette.filter, cmd = m.palette.filter.Update(msg)
	m.palette.applyFilter()
	return m, cmd
}

func (m Model) executeCommand(id string) (tea.Model, tea.Cmd) {
	switch id {
	case "open-shop":
		return m, openInBrowser(m.general.shopURL)
	case "open-admin":
		return m, openInBrowser(m.general.adminURL)
	case "cache-clear":
		return m, m.runCacheClear()
	case "tab-logs":
		m.activeTab = tabLogs
	case "tab-general":
		m.activeTab = tabGeneral
	case "quit":
		m.logs.StopStreaming()
		if m.dockerMode {
			m.overlay = overlayStopConfirm
			m.stopConfirmYes = true
			return m, nil
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) runCacheClear() tea.Cmd {
	e := m.executor
	return func() tea.Msg {
		cmd := e.ConsoleCommand(context.Background(), "cache:clear")
		_ = cmd.Run()
		return nil
	}
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

func (m Model) updateInstallPrompt(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case keyQ, keyCtrlC:
		return m, tea.Quit
	}

	switch m.install.step {
	case installStepAsk:
		switch msg.String() {
		case "left", "h":
			m.install.confirmYes = true
		case "right", "l":
			m.install.confirmYes = false
		case keyTab:
			m.install.confirmYes = !m.install.confirmYes
		case keyEnter:
			if m.install.confirmYes {
				m.install.step = installStepLanguage
				m.install.cursor = 0
			} else {
				m.overlay = overlayNone
				return m, m.startDashboard()
			}
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
			m.install.username.SetValue(defaultUsername)
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
			m.installProg = installProgress{
				spinner: spinner.New(
					spinner.WithSpinner(spinner.Dot),
					spinner.WithStyle(lipgloss.NewStyle().Foreground(tui.BrandColor)),
				),
				progress: progress.New(
					progress.WithColors(tui.BrandColor),
					progress.WithWidth(tui.PhaseCardWidth-15),
					progress.WithoutPercentage(),
				),
			}
			return m, tea.Batch(m.installProg.spinner.Tick, m.runShopwareInstall())
		default:
			var cmd tea.Cmd
			m.install.password, cmd = m.install.password.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

func (m Model) View() tea.View {
	if m.width == 0 || m.height == 0 {
		return tea.NewView("")
	}

	v := tea.NewView("")
	v.AltScreen = true

	switch m.overlay {
	case overlayNone:
		v.Content = m.renderDashboard()
	case overlayCommandPalette:
		v.Content = m.palette.view(m.width, m.height)
	case overlayStarting, overlayStopConfirm, overlayStopping, overlayInstallPrompt, overlayInstalling:
		v.Content = m.renderOverlay()
	}

	return v
}

func (m Model) renderDashboard() string {
	tabHeader := buildTabHeader(int(m.activeTab), m.width)
	footer := m.renderDashboardFooter()

	footerHeight := lipgloss.Height(footer)
	boxHeight := m.height - 3 - footerHeight

	padV := 1
	padH := 3
	if m.activeTab == tabLogs {
		padV = 0
		padH = 1
	}

	contentH := boxHeight - padV*2 - 1
	contentW := m.width - padH*2 - 2

	var content string
	switch m.activeTab {
	case tabGeneral:
		content = m.general.View(m.width, boxHeight)
	case tabLogs:
		m.logs.SetSize(contentW, contentH)
		content = m.logs.View()
	}

	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderTop(false).
		BorderForeground(tui.BorderColor).
		Padding(padV, padH).
		Width(m.width).
		Height(boxHeight)

	return tabHeader + "\n" + contentBox.Render(content) + "\n" + footer
}

func (m Model) renderDashboardFooter() string {
	if m.activeTab == tabLogs {
		followState := "Follow"
		shortcuts := []tui.Shortcut{
			{Key: "↑/↓", Label: "Move cursor"},
			{Key: "enter", Label: "Open source"},
			{Key: "f", Label: followState},
			{Key: "tab", Label: "Next tab"},
			{Key: "ctrl+c", Label: "Exit"},
		}
		return tui.ShortcutBar(shortcuts...)
	}

	return tui.ShortcutBar(
		tui.Shortcut{Key: "ctrl+p", Label: "Commands"},
		tui.Shortcut{Key: "tab", Label: "Next tab"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	)
}

func (m Model) renderOverlay() string {
	var content strings.Builder
	var footerHint string

	switch m.overlay {
	case overlayStarting:
		footerHint = tui.ShortcutBadge("l", "Toggle logs")
		if m.dockerShowLogs {
			return m.renderDockerLogs("Starting Docker containers...", footerHint)
		}
		cardContent := fmt.Sprintf("%s Starting Docker containers...", m.dockerSpinner.View())
		content.WriteString(tui.RenderPhaseCard(cardContent))
	case overlayStopConfirm:
		var card strings.Builder
		warnStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor)
		card.WriteString(warnStyle.Render("Stop Docker containers?"))
		card.WriteString("\n")
		card.WriteString(tui.DimStyle.Render("Do you want to stop the running Docker containers?\nThey can be restarted with shopware-cli project dev."))
		card.WriteString("\n\n")
		activeBtn := lipgloss.NewStyle().
			Foreground(tui.TextColor).
			Background(tui.BrandColor).
			Padding(0, 2)
		inactiveBtn := lipgloss.NewStyle().
			Foreground(tui.MutedColor).
			Background(tui.SubtleBgColor).
			Padding(0, 2)
		var yes, no string
		if m.stopConfirmYes {
			yes = activeBtn.Render("Yes, stop")
			no = inactiveBtn.Render("No, quit")
		} else {
			yes = inactiveBtn.Render("Yes, stop")
			no = activeBtn.Render("No, quit")
		}
		card.WriteString(yes + "  " + no)
		content.WriteString(tui.RenderPhaseCard(card.String()))
		footerHint = tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case overlayStopping:
		footerHint = tui.ShortcutBadge("l", "Toggle logs")
		if m.dockerShowLogs {
			return m.renderDockerLogs("Stopping Docker containers...", footerHint)
		}
		cardContent := fmt.Sprintf("%s Stopping Docker containers...", m.dockerSpinner.View())
		content.WriteString(tui.RenderPhaseCard(cardContent))
	case overlayInstallPrompt:
		var card strings.Builder
		m.renderInstallPrompt(&card)
		content.WriteString(tui.RenderPhaseCard(card.String()))
		footerHint = m.installFooterHint()
	case overlayInstalling:
		if m.installProg.showLogs {
			footerHint = tui.ShortcutBadge("l", "Toggle logs")
			return m.renderDockerLogs("Installing Shopware...", footerHint)
		}
		var card strings.Builder
		total := len(installStepPatterns)
		pctText := fmt.Sprintf(" %d%%", int(float64(m.installProg.currentStep)/float64(total)*100))
		card.WriteString(m.installProg.progress.View() + tui.DimStyle.Render(pctText) + "\n\n")

		for i, sp := range installStepPatterns {
			switch {
			case i < m.installProg.currentStep:
				card.WriteString(tui.StepDone(sp.label))
			case i == m.installProg.currentStep && !m.installProg.done:
				card.WriteString(tui.StepActive(m.installProg.spinner.View(), sp.label))
			case i == m.installProg.currentStep && m.installProg.done:
				card.WriteString(tui.StepDone(sp.label))
			default:
				card.WriteString(tui.StepPending(tui.DimStyle.Render(sp.label)))
			}
		}
		content.WriteString(tui.RenderPhaseCard(strings.TrimRight(card.String(), "\n")))
		footerHint = tui.ShortcutBadge("l", "Toggle logs")
	case overlayNone, overlayCommandPalette:
	}

	return renderPhaseLayout(content.String(), m.width, m.height, footerHint)
}

// renderPhaseLayout renders a full-screen phase view: branding line at top,
// content centered in a bordered box, shortcut footer at bottom.
func renderPhaseLayout(content string, width, height int, footerHint string) string {
	branding := tui.BrandingLine()
	fill := width - tui.BrandingLineWidth()
	if fill < 0 {
		fill = 0
	}
	header := strings.Repeat(" ", fill) + branding

	exit := tui.ShortcutBadge("ctrl+c", "Exit")
	var footer string
	if footerHint != "" {
		sep := lipgloss.NewStyle().Foreground(tui.BorderColor).Render("  │  ")
		footer = footerHint + sep + exit
	} else {
		footer = exit
	}

	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	boxHeight := height - headerHeight - footerHeight

	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BorderColor).
		Padding(1, 3).
		Width(width).
		Height(boxHeight).
		AlignVertical(lipgloss.Center).
		AlignHorizontal(lipgloss.Center)

	contentWidth := lipgloss.Width(content)
	normalized := lipgloss.NewStyle().Width(contentWidth).Render(content)

	return header + "\n" + contentBox.Render(normalized) + "\n" + footer
}

// renderDockerLogs renders a full-screen log view without the mascot card.
func (m Model) renderDockerLogs(title, footerHint string) string {
	branding := tui.BrandingLine()
	fill := m.width - tui.BrandingLineWidth()
	if fill < 0 {
		fill = 0
	}
	header := strings.Repeat(" ", fill) + branding

	sep := lipgloss.NewStyle().Foreground(tui.BorderColor).Render("  │  ")
	footer := footerHint + sep + tui.ShortcutBadge("ctrl+c", "Exit")

	headerHeight := lipgloss.Height(header)
	footerHeight := lipgloss.Height(footer)
	boxHeight := m.height - headerHeight - footerHeight
	// border (2) + padding (2) + title (1) + blank (1) = 6 lines overhead
	visibleLines := boxHeight - 6
	if visibleLines < 1 {
		visibleLines = 1
	}

	var body strings.Builder
	body.WriteString(panelHeaderStyle.Render(title))
	body.WriteString("\n\n")

	start := 0
	if len(m.overlayLines) > visibleLines {
		start = len(m.overlayLines) - visibleLines
	}
	for _, line := range m.overlayLines[start:] {
		body.WriteString(line + "\n")
	}
	if len(m.overlayLines) == 0 {
		body.WriteString(helpStyle.Render("Waiting for command output..."))
	}

	contentBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(tui.BorderColor).
		Padding(1, 3).
		Width(m.width).
		Height(boxHeight)

	return header + "\n" + contentBox.Render(body.String()) + "\n" + footer
}

// overlayMaxLines returns the maximum number of log lines that fit in the overlay.
func (m Model) overlayMaxLines() int {
	if m.height <= 0 {
		return 10
	}
	// Account for border (2), padding (2), title (1), blank line after title (1)
	const overhead = 6
	maxLines := m.height - 2 - overhead
	if maxLines < 10 {
		return 10
	}
	return maxLines
}

func (m Model) renderInstallPrompt(b *strings.Builder) {
	switch m.install.step {
	case installStepAsk:
		warnStyle := lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor)
		b.WriteString(warnStyle.Render("Shopware is not installed"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("This project has not been set up yet. The installation\nwill create the database, run migrations and configure\nyour local development environment."))
		b.WriteString("\n\n")
		activeBtn := lipgloss.NewStyle().
			Foreground(tui.TextColor).
			Background(tui.BrandColor).
			Padding(0, 2)
		inactiveBtn := lipgloss.NewStyle().
			Foreground(tui.MutedColor).
			Background(tui.SubtleBgColor).
			Padding(0, 2)
		var yes, no string
		if m.install.confirmYes {
			yes = activeBtn.Render("Yes, install now")
			no = inactiveBtn.Render("No, skip")
		} else {
			yes = inactiveBtn.Render("Yes, install now")
			no = activeBtn.Render("No, skip")
		}
		b.WriteString(yes + "  " + no)

	case installStepLanguage:
		b.WriteString(tui.TextBadge("Step 1/4"))
		b.WriteString("\n\n")
		opts := make([]tui.SelectOption, len(installLanguages))
		for i, lang := range installLanguages {
			opts[i] = tui.SelectOption{Label: lang.label, Detail: lang.id}
		}
		b.WriteString(tui.RenderSelectList("Default Language", "Select the primary language for your storefront", opts, m.install.cursor))

	case installStepCurrency:
		b.WriteString(tui.TextBadge("Step 2/4"))
		b.WriteString("\n\n")
		opts := make([]tui.SelectOption, len(installCurrencies))
		for i, curr := range installCurrencies {
			opts[i] = tui.SelectOption{Label: curr}
		}
		b.WriteString(tui.RenderSelectList("Default Currency", "Select the default currency for pricing", opts, m.install.cursor))

	case installStepUsername:
		b.WriteString(tui.TextBadge("Step 3/4"))
		b.WriteString("\n\n")
		b.WriteString(tui.TitleStyle.Render("Admin Username"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Enter the username for the admin account"))
		b.WriteString("\n\n")
		b.WriteString(m.install.username.View())

	case installStepPassword:
		b.WriteString(tui.TextBadge("Step 4/4"))
		b.WriteString("\n\n")
		b.WriteString(tui.TitleStyle.Render("Admin Password"))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Enter the password for the admin account"))
		b.WriteString("\n\n")
		b.WriteString(m.install.password.View())
	}
}

func (m Model) installFooterHint() string {
	switch m.install.step {
	case installStepAsk:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "←/→", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case installStepLanguage, installStepCurrency:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "↑/↓", Label: "Select"},
			tui.Shortcut{Key: "enter", Label: "Confirm"},
		)
	case installStepUsername:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "enter", Label: "Continue"},
		)
	case installStepPassword:
		return tui.ShortcutBar(
			tui.Shortcut{Key: "enter", Label: "Install"},
		)
	}
	return ""
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
		withEnv := e.WithEnv(map[string]string{
			"INSTALL_LOCALE":         language,
			"INSTALL_CURRENCY":       currency,
			"INSTALL_ADMIN_USERNAME": username,
			"INSTALL_ADMIN_PASSWORD": password,
		})
		cmd := withEnv.PHPCommand(context.Background(), "vendor/bin/shopware-deployment-helper", "run")

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

package devtui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	endoflife "github.com/shyim/go-endoflife-api"

	dockerpkg "github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

// Receiver convention for the tab models (Overview, Instance, Config):
// methods that return the model (Update/handleKey/activate/View and other
// pure reads) use value receivers; methods that mutate the model in place and
// return nothing or only a tea.Cmd (SetSize, start*/stop* streaming helpers)
// use pointer receivers.
type OverviewModel struct {
	envType            string
	shopURL            string
	adminURL           string
	username           string
	password           string
	services           []DiscoveredService
	background         []BackgroundProcess
	projectRoot        string
	executor           executor.Executor
	shopCfg            *shop.Config
	loading            bool
	err                error
	width              int
	height             int
	adminWatchRunning  bool
	adminWatchStarting bool
	sfWatchRunning     bool
	sfWatchStarting    bool
	shopwareVersion    string
	securityEnd        time.Time
	health             []healthCheck
	healthLoading      bool
	cursor             int // focus index: 0=Admin watcher, 1=Storefront watcher
}

type DiscoveredService struct {
	Name     string
	URL      string
	Username string
	Password string
}

// BackgroundProcess is a long-running compose service without a published port
// (the messenger worker and scheduled-task runner). Running reflects whether its
// container is currently up.
type BackgroundProcess struct {
	Name    string
	Running bool
}

type servicesLoadedMsg struct {
	services   []DiscoveredService
	background []BackgroundProcess
	webPort    int
	err        error
}

type shopwareVersionLoadedMsg struct {
	version string
}

type securityEndLoadedMsg struct {
	securityEnd time.Time
}

// watcherHandle is shared between the goroutine running a watcher's preparation
// steps and the UI model. The goroutine stores the dev-server process on it once
// preparation succeeds, so the model can stop it later. Stopping before that
// cancels the preparation context so an in-flight prepare does not start an
// orphan dev server after the UI has marked the watcher as stopped.
type watcherHandle struct {
	mu      sync.Mutex
	cancel  context.CancelFunc
	process *executor.Process
	stopped bool
}

// begin returns the cancellable context for the preparation steps. If the
// watcher was already stopped before preparation started, the returned context
// is already cancelled.
func (h *watcherHandle) begin(parent context.Context) context.Context {
	ctx, cancel := context.WithCancel(parent)
	h.mu.Lock()
	h.cancel = cancel
	stopped := h.stopped
	h.mu.Unlock()
	if stopped {
		cancel()
	}
	return ctx
}

// set stores the started dev-server process. It reports whether the watcher was
// already stopped, in which case the caller must not keep the process running.
func (h *watcherHandle) set(p *executor.Process) (alreadyStopped bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.stopped {
		return true
	}
	h.process = p
	return false
}

func (h *watcherHandle) isStopped() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stopped
}

func (h *watcherHandle) stop(ctx context.Context) {
	h.mu.Lock()
	h.stopped = true
	p := h.process
	cancel := h.cancel
	h.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if p != nil {
		_ = p.Stop(ctx)
	}
}

type watcherStartedMsg struct {
	name   string
	handle *watcherHandle
	lines  <-chan string
}
type watcherStoppedMsg struct {
	name string
	err  error
}

// watcherRunningMsg is emitted when a watcher's preparation completes and the
// dev-server process is about to start. err is non-nil if preparation failed.
type watcherRunningMsg struct {
	name string
	err  error
}

type knownService struct {
	Name       string
	TargetPort int
	Username   string
	Password   string
}

var knownServices = map[string]knownService{
	"adminer":  {Name: "Adminer", TargetPort: 8080, Username: "root", Password: "root"},
	"mailer":   {Name: "Mailpit", TargetPort: 8025},
	"lavinmq":  {Name: "Queue (LavinMQ)", TargetPort: 15672, Username: "guest", Password: "guest"},
	"rabbitmq": {Name: "Queue (RabbitMQ)", TargetPort: 15672, Username: "guest", Password: "guest"},
}

var ignoredServices = map[string]bool{
	"web":      true,
	"database": true,
}

// backgroundServiceLabel returns the display label for a compose service that is
// one of the dedicated background processes (defined once in internal/docker),
// and whether the service is such a process. These have no published port, so
// they never appear in the Services list and are surfaced in the "Background
// processing" section instead.
func backgroundServiceLabel(service string) (string, bool) {
	for _, bg := range dockerpkg.BackgroundServices {
		if bg.Name == service {
			return bg.Label, true
		}
	}
	return "", false
}

// webServiceTargetPort is the container port the Shopware web server (the "web"
// service) listens on. Its published host port determines the shop URL port.
const webServiceTargetPort = 8000

// linkURL renders url as a clickable OSC 8 hyperlink in the shared link style.
// Terminals without hyperlink support show the plain styled URL instead.
func linkURL(url string) string {
	if url == "" {
		return ""
	}
	return tui.RenderStyledLink(url)
}

// deriveAdminURL returns the admin URL for the given shop URL by appending the
// "admin" path segment.
func deriveAdminURL(shopURL string) string {
	adminURL := shopURL
	if adminURL != "" && !strings.HasSuffix(adminURL, "/") {
		adminURL += "/"
	}
	return adminURL + "admin"
}

func NewOverviewModel(envType, shopURL, username, password, projectRoot string, exec executor.Executor, shopCfg *shop.Config) OverviewModel {
	return OverviewModel{
		envType:       envType,
		shopURL:       shopURL,
		adminURL:      deriveAdminURL(shopURL),
		username:      username,
		password:      password,
		projectRoot:   projectRoot,
		executor:      exec,
		shopCfg:       shopCfg,
		loading:       true,
		healthLoading: true,
	}
}

func (m OverviewModel) Init() tea.Cmd {
	return tea.Batch(
		discoverServices(m.projectRoot),
		loadShopwareVersion(m.projectRoot),
		loadSetupHealth(m.projectRoot, m.executor),
	)
}

func (m *OverviewModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

type browserOpenedMsg struct{}

// stopWatcherRequestMsg flows from OverviewModel to Model so the parent can
// call stopWatcher (which needs access to the logs model and watcher map).
type stopWatcherRequestMsg struct{ name string }

// startStorefrontWatchRequestMsg flows from OverviewModel to Model so the parent
// can open the sales-channel picker (which needs the executor) before starting
// the storefront watcher, matching the command-palette flow.
type startStorefrontWatchRequestMsg struct{}

func (m OverviewModel) Update(msg tea.Msg) (OverviewModel, tea.Cmd) {
	switch msg := msg.(type) {
	case servicesLoadedMsg:
		m.loading = false
		m.services = msg.services
		m.background = msg.background
		m.err = msg.err
		if msg.webPort != 0 {
			m.shopURL = ResolveShopURL(m.shopURL, msg.webPort)
			m.adminURL = deriveAdminURL(m.shopURL)
		}
	case shopwareVersionLoadedMsg:
		m.shopwareVersion = msg.version
		if msg.version != "" {
			return m, loadSecurityEnd(msg.version)
		}
	case securityEndLoadedMsg:
		m.securityEnd = msg.securityEnd
	case setupHealthLoadedMsg:
		m.healthLoading = false
		m.health = msg.checks
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m OverviewModel) focusCount() int {
	// Admin watcher + Storefront watcher
	return 2
}

func (m OverviewModel) handleKey(msg tea.KeyPressMsg) (OverviewModel, tea.Cmd) {
	count := m.focusCount()
	if count == 0 {
		return m, nil
	}

	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < count-1 {
			m.cursor++
		}
	case "enter":
		return m.activate()
	}
	return m, nil
}

func (m OverviewModel) activate() (OverviewModel, tea.Cmd) {
	switch m.cursor {
	case 0: // Admin watcher
		if m.adminWatchRunning {
			return m, func() tea.Msg { return stopWatcherRequestMsg{name: watcherAdmin} }
		}
		if !m.adminWatchStarting {
			m.adminWatchStarting = true
			return m, m.startAdminWatch()
		}
	case 1: // Storefront watcher
		if m.sfWatchRunning {
			return m, func() tea.Msg { return stopWatcherRequestMsg{name: watcherStorefront} }
		}
		if !m.sfWatchStarting {
			return m, func() tea.Msg { return startStorefrontWatchRequestMsg{} }
		}
	}
	return m, nil
}

func openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		_ = system.OpenURL(context.Background(), url)
		return browserOpenedMsg{}
	}
}

// overviewTwoColumnMinWidth is the tab width below which the overview falls
// back to a single stacked column instead of the report/user-action split.
const overviewTwoColumnMinWidth = 100

// overviewRightColumnWidth is the inner width of the "User action" column.
const overviewRightColumnWidth = 32

func (m OverviewModel) View(width, height int) string {
	usable := width - 8
	if width < overviewTwoColumnMinWidth {
		return m.renderStacked(usable)
	}

	leftWidth := usable - overviewRightColumnWidth - 3

	left := lipgloss.NewStyle().Width(leftWidth).Render(m.renderProjectReport(leftWidth))
	right := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(tui.BorderColor).
		PaddingLeft(2).
		Height(lipgloss.Height(left)).
		Render(m.renderUserActions())

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

// renderProjectReport renders the left column: the readonly project details
// and setup report.
func (m OverviewModel) renderProjectReport(width int) string {
	divider := tui.SectionDivider(width)

	var s strings.Builder
	s.WriteString(helpStyle.Render("Project details and readonly setup report."))
	s.WriteString("\n\n")
	s.WriteString(m.renderShopSection())
	s.WriteString(divider)
	s.WriteString(m.renderAccess())
	if len(m.background) > 0 {
		s.WriteString(divider)
		s.WriteString(tui.TitleStyle.Render("Background processing"))
		s.WriteString("\n")
		s.WriteString(m.renderBackgroundProcesses())
	}
	s.WriteString(divider)
	s.WriteString(m.renderSetupHealth())
	return s.String()
}

// renderUserActions renders the right column: everything the user can act on.
func (m OverviewModel) renderUserActions() string {
	var s strings.Builder
	s.WriteString(tui.SectionTitleStyle.Render("User action"))
	s.WriteString("\n\n")
	s.WriteString(m.renderWatchers())
	return s.String()
}

// renderStacked is the single-column fallback for narrow terminals, keeping
// every section of the two-column layout.
func (m OverviewModel) renderStacked(width int) string {
	divider := tui.SectionDivider(width)

	var s strings.Builder
	s.WriteString(helpStyle.Render("Project details and readonly setup report."))
	s.WriteString("\n\n")
	s.WriteString(m.renderShopSection())
	s.WriteString(divider)
	s.WriteString(m.renderAccess())
	if len(m.background) > 0 {
		s.WriteString(divider)
		s.WriteString(tui.TitleStyle.Render("Background processing"))
		s.WriteString("\n")
		s.WriteString(m.renderBackgroundProcesses())
	}
	s.WriteString(divider)
	s.WriteString(m.renderWatchers())
	s.WriteString(divider)
	s.WriteString(m.renderSetupHealth())
	return s.String()
}

func (m OverviewModel) renderShopSection() string {
	var s strings.Builder
	s.WriteString(tui.TitleStyle.Render("Shop"))
	s.WriteString("\n")
	if m.shopwareVersion != "" {
		s.WriteString(tui.KVRow("Version", valueStyle.Render(m.shopwareVersion)))
	}
	if !m.securityEnd.IsZero() {
		s.WriteString(tui.KVRow("Security updates", renderSecurityEnd(m.securityEnd, time.Now())))
	}
	s.WriteString(tui.KVRow("Environment", activeBadgeStyle.Render(m.envType)))
	s.WriteString(tui.KVRow("Shop URL", linkURL(m.shopURL)))
	s.WriteString(tui.KVRow("Admin URL", linkURL(m.adminURL)))
	return s.String()
}

// accessRow is one line of the Access table: a reachable service with its URL
// and credentials. noAuth marks services that are open without credentials, as
// opposed to credentials that are simply not known yet.
type accessRow struct {
	name     string
	url      string
	username string
	password string
	noAuth   bool
}

// renderAccess renders the Access table: the Shop Admin login first, followed
// by every discovered auxiliary service with its credentials.
func (m OverviewModel) renderAccess() string {
	rows := []accessRow{{
		name:     "Shop Admin",
		url:      m.adminURL,
		username: m.username,
		password: m.password,
	}}
	for _, service := range m.services {
		rows = append(rows, accessRow{
			name:     service.Name,
			url:      service.URL,
			username: service.Username,
			password: service.Password,
			noAuth:   service.Username == "" && service.Password == "",
		})
	}

	serviceWidth, urlWidth, userWidth := lipgloss.Width("Service"), lipgloss.Width("URL"), lipgloss.Width("Username")
	for _, row := range rows {
		serviceWidth = max(serviceWidth, lipgloss.Width(row.name))
		urlWidth = max(urlWidth, lipgloss.Width(row.url))
		userWidth = max(userWidth, lipgloss.Width(row.username))
	}
	serviceStyle := lipgloss.NewStyle().Width(serviceWidth + 3)
	urlStyleW := lipgloss.NewStyle().Width(urlWidth + 3)
	userStyle := lipgloss.NewStyle().Width(userWidth + 3)

	var s strings.Builder
	s.WriteString(tui.TitleStyle.Render("Access"))
	s.WriteString("\n")

	dim := lipgloss.NewStyle().Foreground(tui.MutedColor)
	s.WriteString("  ")
	s.WriteString(serviceStyle.Render(dim.Render("Service")))
	s.WriteString(urlStyleW.Render(dim.Render("URL")))
	s.WriteString(userStyle.Render(dim.Render("Username")))
	s.WriteString(dim.Render("Password / Auth"))
	s.WriteString("\n")

	for _, row := range rows {
		username := tui.DimStyle.Render("-")
		if row.username != "" {
			username = valueStyle.Render(row.username)
		}

		auth := tui.DimStyle.Render("-")
		switch {
		case row.noAuth:
			auth = tui.DimStyle.Render("no auth")
		case row.password != "":
			auth = secretStyle.Render(row.password)
		}

		s.WriteString("  ")
		s.WriteString(serviceStyle.Render(row.name))
		s.WriteString(urlStyleW.Render(linkURL(row.url)))
		s.WriteString(userStyle.Render(username))
		s.WriteString(auth)
		s.WriteString("\n")
	}

	switch {
	case m.loading:
		s.WriteString("  " + helpStyle.Render("Scanning for further local services...") + "\n")
	case m.err != nil:
		s.WriteString("  " + errorStyle.Render(m.err.Error()) + "\n")
	}
	if m.username == "" && m.password == "" {
		s.WriteString("  " + helpStyle.Render("Admin credentials will appear here once Shopware is installed.") + "\n")
	}

	return s.String()
}

func (m OverviewModel) renderWatchers() string {
	var s strings.Builder
	s.WriteString(tui.TitleStyle.Render("Watchers"))
	s.WriteString("\n")
	s.WriteString(m.renderWatcherStatus("Admin", m.adminWatchRunning, m.adminWatchStarting, "http://127.0.0.1:5173", m.cursor == 0))
	s.WriteString(m.renderWatcherStatus("Storefront", m.sfWatchRunning, m.sfWatchStarting, "http://127.0.0.1:9998", m.cursor == 1))
	return s.String()
}

func (m OverviewModel) renderBackgroundProcesses() string {
	nameWidth := 0
	for _, proc := range m.background {
		nameWidth = max(nameWidth, lipgloss.Width(proc.Name))
	}
	nameStyle := lipgloss.NewStyle().Width(nameWidth + 3)

	var s strings.Builder
	for _, proc := range m.background {
		dot := lipgloss.NewStyle().Foreground(tui.SuccessColor).Render("●")
		status := lipgloss.NewStyle().Foreground(tui.SuccessColor).Render("running")
		if !proc.Running {
			dot = tui.DimStyle.Render("●")
			status = tui.DimStyle.Render("stopped")
		}
		s.WriteString(fmt.Sprintf("  %s %s%s\n", dot, nameStyle.Render(proc.Name), status))
	}
	return s.String()
}

func (m OverviewModel) startAdminWatch() tea.Cmd {
	e := m.executor
	projectRoot := m.projectRoot
	shopCfg := m.shopCfg

	return startWatcher(watcherAdmin, func(ctx context.Context, out io.Writer) (*executor.Process, error) {
		logStep(out, "Preparing plugins.json...")
		if err := extension.WriteProjectPluginJson(ctx, projectRoot, shopCfg, e); err != nil {
			return nil, fmt.Errorf("preparing plugins.json: %w", err)
		}

		watchProcess, err := extension.PrepareAdminWatcher(ctx, projectRoot, e, out)
		if err != nil {
			return nil, fmt.Errorf("starting admin watcher: %w", err)
		}

		return watchProcess, nil
	})
}

func (m OverviewModel) startStorefrontWatch(opts extension.StorefrontWatcherOptions) tea.Cmd {
	e := m.executor
	projectRoot := m.projectRoot
	shopCfg := m.shopCfg

	return startWatcher(watcherStorefront, func(ctx context.Context, out io.Writer) (*executor.Process, error) {
		logStep(out, "Preparing plugins.json...")
		if err := extension.WriteProjectPluginJson(ctx, projectRoot, shopCfg, e); err != nil {
			return nil, fmt.Errorf("preparing plugins.json: %w", err)
		}

		watchProcess, err := extension.PrepareStorefrontWatcher(ctx, projectRoot, e, opts, out)
		if err != nil {
			return nil, fmt.Errorf("starting storefront watcher: %w", err)
		}

		return watchProcess, nil
	})
}

// logStep mirrors extension.logStep for prep work done inside devtui itself.
func logStep(out io.Writer, msg string) {
	_, _ = fmt.Fprintf(out, "\n> %s\n", msg)
}

// startWatcher runs a watcher's preparation steps in the background, streaming
// all of their output (including npm install) into a line channel that is shown
// live in the Logs tab. The watcher process started by prepare is streamed into
// the same channel and stored on the returned handle so it can be stopped later.
//
// It returns a batch of two commands: one emits watcherStartedMsg immediately
// (so log streaming begins), and another emits watcherRunningMsg once the
// preparation goroutine signals that the dev-server process is starting (or
// preparation failed). This keeps the UI in a visible "starting" state during
// preparation rather than flipping to "running" instantly.
func startWatcher(name string, prepare func(ctx context.Context, out io.Writer) (*executor.Process, error)) tea.Cmd {
	handle := &watcherHandle{}
	lines := make(chan string, streamBufferSize)
	running := make(chan error, 1) // buffered so the goroutine never blocks

	startedCmd := func() tea.Msg {
		go func() {
			defer close(lines)

			ctx := handle.begin(logging.DisableLogger(context.Background()))
			pr, pw := io.Pipe()

			scanDone := make(chan struct{})
			go func() {
				defer close(scanDone)
				scanner := bufio.NewScanner(pr)
				scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
				for scanner.Scan() {
					lines <- scanner.Text()
				}
			}()

			process, err := prepare(ctx, pw)
			// Stop streaming the preparation output before handling the process so
			// the prep scanner drains and following process output stays ordered.
			_ = pw.Close()
			<-scanDone

			if err != nil {
				lines <- errorStyle.Render(err.Error())
				running <- err
				return
			}

			// If the user stopped the watcher while prepare was running, do not
			// keep the freshly started dev server around as an orphan.
			if handle.set(process) {
				stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = process.Stop(stopCtx)
				cancel()
				running <- fmt.Errorf("watcher stopped")
				return
			}

			lines <- helpStyle.Render("> Starting watcher...")
			running <- nil // signal: preparation succeeded, process is starting

			stdout, err := process.StartCombined()
			if err != nil {
				lines <- errorStyle.Render(err.Error())
				return
			}

			scanner := bufio.NewScanner(stdout)
			scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
			for scanner.Scan() {
				lines <- scanner.Text()
			}
			// Surface a non-zero exit (e.g. the dev server crashing on a busy
			// port) unless the user stopped the watcher, where the signal-induced
			// exit error is expected.
			if runErr := process.Wait(); runErr != nil && !handle.isStopped() {
				lines <- errorStyle.Render(runErr.Error())
			}
		}()

		return watcherStartedMsg{name: name, handle: handle, lines: lines}
	}

	runningCmd := func() tea.Msg {
		err := <-running
		return watcherRunningMsg{name: name, err: err}
	}

	return tea.Batch(startedCmd, runningCmd)
}

func (m OverviewModel) renderWatcherStatus(label string, running, starting bool, url string, focused bool) string {
	var checkbox, status string
	switch {
	case running:
		if focused {
			checkbox = lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor).Render("[x]")
		} else {
			checkbox = lipgloss.NewStyle().Render("[x]")
		}
		status = lipgloss.NewStyle().Bold(true).Render("running")
	case starting:
		checkbox = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("[~]")
		status = lipgloss.NewStyle().Foreground(tui.BrandColor).Render("starting...")
	default:
		if focused {
			checkbox = lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor).Render("[ ]")
		} else {
			checkbox = tui.DimStyle.Render("[ ]")
		}
		status = tui.DimStyle.Render("stopped")
	}

	row := fmt.Sprintf("  %s %s%s\n", checkbox, lipgloss.NewStyle().Width(14).Render(label), status)
	if running && url != "" {
		row += "      " + linkURL(url) + "\n"
	}
	return row
}

type dockerComposePSOutput struct {
	Name       string `json:"Name"`
	Service    string `json:"Service"`
	State      string `json:"State"`
	Publishers []struct {
		URL           string `json:"URL"`
		TargetPort    int    `json:"TargetPort"`
		PublishedPort int    `json:"PublishedPort"`
		Protocol      string `json:"Protocol"`
	} `json:"Publishers"`
}

// DiscoverServices returns the auxiliary services published by the running
// docker development environment.
func DiscoverServices(ctx context.Context, projectRoot string) ([]DiscoveredService, error) {
	services, _, err := DiscoverComposeServices(ctx, projectRoot)
	return services, err
}

// DiscoverComposeServices parses `docker compose ps` once and returns both the
// auxiliary services and the host port the web service's HTTP port (8000) is
// published on. webPort is 0 when it cannot be determined (e.g. the environment
// is down or the web container does not publish port 8000).
func DiscoverComposeServices(ctx context.Context, projectRoot string) (services []DiscoveredService, webPort int, err error) {
	services, _, webPort, err = discoverCompose(ctx, projectRoot)
	return services, webPort, err
}

// discoverCompose parses `docker compose ps --format json` once and classifies
// every container into a published auxiliary service, a background process, or
// (for web) the shop's published port.
func discoverCompose(ctx context.Context, projectRoot string) (services []DiscoveredService, background []BackgroundProcess, webPort int, err error) {
	cmd := composeCommand(ctx, projectRoot, "ps", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return nil, nil, 0, fmt.Errorf("docker compose ps: %w", err)
	}

	type containerInfo struct {
		service    string
		publishers map[int]int
	}
	var containers []containerInfo

	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}

		var container dockerComposePSOutput
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			continue
		}

		if label, ok := backgroundServiceLabel(container.Service); ok {
			background = append(background, BackgroundProcess{
				Name:    label,
				Running: container.State == "running",
			})
			continue
		}

		ports := make(map[int]int)
		for _, pub := range container.Publishers {
			if pub.PublishedPort != 0 {
				ports[pub.TargetPort] = pub.PublishedPort
			}
		}

		if container.Service == "web" {
			if port, ok := ports[webServiceTargetPort]; ok {
				webPort = port
			}
		}

		if len(ports) > 0 {
			containers = append(containers, containerInfo{
				service:    container.Service,
				publishers: ports,
			})
		}
	}

	for _, c := range containers {
		if ignoredServices[c.service] {
			continue
		}

		known, ok := knownServices[c.service]
		if !ok {
			continue
		}

		publishedPort, hasPort := c.publishers[known.TargetPort]
		if !hasPort {
			continue
		}

		services = append(services, DiscoveredService{
			Name:     known.Name,
			URL:      fmt.Sprintf("http://127.0.0.1:%d", publishedPort),
			Username: known.Username,
			Password: known.Password,
		})
	}

	return services, background, webPort, nil
}

// ResolveShopURL rewrites the port in shopURL to webPort, the host port the web
// container is actually published on. The configured URL is returned unchanged
// when shopURL is empty, webPort is 0, or shopURL cannot be parsed.
func ResolveShopURL(shopURL string, webPort int) string {
	if shopURL == "" || webPort == 0 {
		return shopURL
	}

	u, err := url.Parse(shopURL)
	if err != nil || u.Host == "" {
		return shopURL
	}

	u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(webPort))
	return u.String()
}

func discoverServices(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		services, background, webPort, err := discoverCompose(context.Background(), projectRoot)
		return servicesLoadedMsg{services: services, background: background, webPort: webPort, err: err}
	}
}

func loadShopwareVersion(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		return shopwareVersionLoadedMsg{version: detectShopwareVersion(projectRoot)}
	}
}

// loadSecurityEnd fetches, for the running Shopware major.minor release, the
// date until which security updates are provided (the end-of-life date reported
// by endoflife.date). It resolves to an empty string when the version cannot be
// reduced to a major.minor cycle or the release is unknown to the API.
func loadSecurityEnd(version string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), securityEndTimeout)
		defer cancel()
		return securityEndLoadedMsg{securityEnd: fetchSecurityEnd(ctx, version)}
	}
}

// securityEndTimeout bounds the endoflife.date lookup so a slow or unreachable
// API cannot leave the lookup hanging.
const securityEndTimeout = 5 * time.Second

func fetchSecurityEnd(ctx context.Context, version string) time.Time {
	release := majorMinor(version)
	if release == "" {
		return time.Time{}
	}

	resp, err := endoflife.NewClient().ProductRelease(ctx, "shopware", release)
	if err != nil || resp.Result.EolFrom == nil {
		return time.Time{}
	}
	return resp.Result.EolFrom.Time
}

// renderSecurityEnd formats the end-of-life date as "until YYYY-MM-DD (N days
// left)", colored by how much runway is left relative to now: green with more
// than a year, yellow within a year, and red within a month or already expired.
func renderSecurityEnd(eol, now time.Time) string {
	text := "until " + eol.Format("2006-01-02") + " (" + securityEndRemaining(eol, now) + ")"

	var c color.Color
	switch securityEndLevel(eol, now) {
	case securityEndCritical:
		c = tui.ErrorColor
	case securityEndWarning:
		c = tui.WarnColor
	case securityEndOK:
		c = tui.SuccessColor
	}

	return lipgloss.NewStyle().Foreground(c).Render(text)
}

// securityEndRemaining returns a human-readable description of the time left
// until eol, counted in whole days: "N days left", "1 day left", "expires
// today", or "expired" once the date has passed.
func securityEndRemaining(eol, now time.Time) string {
	remaining := eol.Sub(now)
	if remaining < 0 {
		return "expired"
	}
	days := int(remaining / (24 * time.Hour))
	switch days {
	case 0:
		return "expires today"
	case 1:
		return "1 day left"
	default:
		return fmt.Sprintf("%d days left", days)
	}
}

type securityEndStatus int

const (
	securityEndOK       securityEndStatus = iota // more than a year of support left
	securityEndWarning                           // less than a year left
	securityEndCritical                          // within a month or already expired
)

// securityEndLevel classifies how urgent the end of security support is: red
// within a month or expired, yellow within a year, green otherwise.
func securityEndLevel(eol, now time.Time) securityEndStatus {
	switch remaining := eol.Sub(now); {
	case remaining < 30*24*time.Hour:
		return securityEndCritical
	case remaining < 365*24*time.Hour:
		return securityEndWarning
	default:
		return securityEndOK
	}
}

// majorMinor reduces a full Shopware version like "6.7.0.0" to its major.minor
// release cycle ("6.7"), which is how endoflife.date identifies releases. It
// returns an empty string when the version does not have at least two segments.
func majorMinor(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "." + parts[1]
}

func detectShopwareVersion(projectRoot string) string {
	lock, err := packagist.ReadComposerLock(filepath.Join(projectRoot, "composer.lock"))
	if err != nil {
		return ""
	}
	for _, name := range []string{"shopware/core", "shopware/platform"} {
		if pkg := lock.GetPackage(name); pkg != nil {
			return strings.TrimPrefix(pkg.Version, "v")
		}
	}
	return ""
}

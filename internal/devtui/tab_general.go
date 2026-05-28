package devtui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

type GeneralModel struct {
	envType            string
	shopURL            string
	adminURL           string
	username           string
	password           string
	services           []DiscoveredService
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
}

type DiscoveredService struct {
	Name     string
	URL      string
	Username string
	Password string
}

type servicesLoadedMsg struct {
	services []DiscoveredService
	err      error
}

type shopwareVersionLoadedMsg struct {
	version string
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

func NewGeneralModel(envType, shopURL, username, password, projectRoot string, exec executor.Executor, shopCfg *shop.Config) GeneralModel {
	adminURL := shopURL
	if adminURL != "" && !strings.HasSuffix(adminURL, "/") {
		adminURL += "/"
	}
	adminURL += "admin"

	return GeneralModel{
		envType:     envType,
		shopURL:     shopURL,
		adminURL:    adminURL,
		username:    username,
		password:    password,
		projectRoot: projectRoot,
		executor:    exec,
		shopCfg:     shopCfg,
		loading:     true,
	}
}

func (m GeneralModel) Init() tea.Cmd {
	return tea.Batch(discoverServices(m.projectRoot), loadShopwareVersion(m.projectRoot))
}

func (m *GeneralModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

type browserOpenedMsg struct{}

func (m GeneralModel) Update(msg tea.Msg) (GeneralModel, tea.Cmd) {
	switch msg := msg.(type) {
	case servicesLoadedMsg:
		m.loading = false
		m.services = msg.services
		m.err = msg.err
	case shopwareVersionLoadedMsg:
		m.shopwareVersion = msg.version
	}
	return m, nil
}

func openInBrowser(url string) tea.Cmd {
	return func() tea.Msg {
		_ = exec.CommandContext(context.Background(), "open", url).Start()
		return browserOpenedMsg{}
	}
}

func (m GeneralModel) View(width, height int) string {
	var s strings.Builder

	divider := tui.SectionDivider(width - 8)

	s.WriteString(tui.TitleStyle.Render("Shop"))
	s.WriteString("\n")
	if m.shopwareVersion != "" {
		s.WriteString(tui.KVRow("Version", valueStyle.Render(m.shopwareVersion)))
	}
	s.WriteString(tui.KVRow("Environment", activeBadgeStyle.Render(m.envType)))
	s.WriteString(tui.KVRow("Shop URL", urlStyle.Render(m.shopURL)))
	s.WriteString(tui.KVRow("Admin URL", urlStyle.Render(m.adminURL)))

	s.WriteString(divider)

	s.WriteString(tui.TitleStyle.Render("Admin Access"))
	s.WriteString("\n")
	if m.username == "" && m.password == "" {
		s.WriteString("  ")
		s.WriteString(helpStyle.Render("Credentials will appear here once Shopware is installed."))
		s.WriteString("\n")
	} else {
		s.WriteString(tui.KVRow("Username", valueStyle.Render(m.username)))
		s.WriteString(tui.KVRow("Password", secretStyle.Render(m.password)))
	}

	s.WriteString(divider)

	s.WriteString(tui.TitleStyle.Render("Watchers"))
	s.WriteString("\n")
	s.WriteString(m.renderWatcherStatus("Admin", m.adminWatchRunning, m.adminWatchStarting, "http://127.0.0.1:5173"))
	s.WriteString(m.renderWatcherStatus("Storefront", m.sfWatchRunning, m.sfWatchStarting, "http://127.0.0.1:9998"))

	s.WriteString(divider)

	s.WriteString(tui.TitleStyle.Render("Services"))
	s.WriteString("\n")
	s.WriteString(m.renderServices())

	return s.String()
}

func (m *GeneralModel) startAdminWatch() tea.Cmd {
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

func (m *GeneralModel) startStorefrontWatch(opts extension.StorefrontWatcherOptions) tea.Cmd {
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
func startWatcher(name string, prepare func(ctx context.Context, out io.Writer) (*executor.Process, error)) tea.Cmd {
	return func() tea.Msg {
		handle := &watcherHandle{}
		lines := make(chan string, streamBufferSize)

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
				return
			}

			// If the user stopped the watcher while prepare was running, do not
			// keep the freshly started dev server around as an orphan.
			if handle.set(process) {
				stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = process.Stop(stopCtx)
				cancel()
				return
			}

			lines <- helpStyle.Render("> Starting watcher...")

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
}

func (m GeneralModel) renderWatcherStatus(label string, running, starting bool, url string) string {
	switch {
	case running:
		row := tui.KVRow(label, activeBadgeStyle.Render("RUNNING"))
		if url != "" {
			row += tui.KVRow("  URL", urlStyle.Render(url))
		}
		return row
	case starting:
		return tui.KVRow(label, tui.StatusBadge("starting", tui.BrandColor))
	default:
		return tui.KVRow(label, helpStyle.Render("stopped"))
	}
}

func (m GeneralModel) renderServices() string {
	switch {
	case m.loading:
		return "  " + tui.StatusBadge("scanning", tui.BrandColor) + "\n" +
			"  " + helpStyle.Render("Looking for published local services.") + "\n"
	case m.err != nil:
		return "  " + tui.StatusBadge("failed", tui.ErrorColor) + "\n" +
			"  " + errorStyle.Render(m.err.Error()) + "\n"
	case len(m.services) == 0:
		return "  " + helpStyle.Render("No auxiliary services detected.") + "\n"
	}

	var s strings.Builder
	for _, service := range m.services {
		s.WriteString(tui.KVRow(service.Name, urlStyle.Render(service.URL)))
		if service.Username != "" {
			s.WriteString(tui.KVRow("  Username", valueStyle.Render(service.Username)))
			s.WriteString(tui.KVRow("  Password", secretStyle.Render(service.Password)))
		}
	}
	return s.String()
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

func DiscoverServices(ctx context.Context, projectRoot string) ([]DiscoveredService, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--format", "json")
	cmd.Dir = projectRoot
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w", err)
	}

	var services []DiscoveredService

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

		ports := make(map[int]int)
		for _, pub := range container.Publishers {
			if pub.PublishedPort != 0 {
				ports[pub.TargetPort] = pub.PublishedPort
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

	return services, nil
}

func discoverServices(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		services, err := DiscoverServices(context.Background(), projectRoot)
		return servicesLoadedMsg{services: services, err: err}
	}
}

func loadShopwareVersion(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		return shopwareVersionLoadedMsg{version: detectShopwareVersion(projectRoot)}
	}
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

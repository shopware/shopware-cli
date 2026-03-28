package devtui

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/tui"
)

type GeneralModel struct {
	envType            string
	shopURL            string
	adminURL           string
	username           string
	password           string
	services           []discoveredService
	projectRoot        string
	executor           executor.Executor
	loading            bool
	err                error
	width              int
	height             int
	adminWatchRunning  bool
	adminWatchStarting bool
	sfWatchRunning     bool
	sfWatchStarting    bool
}

type discoveredService struct {
	Name     string
	URL      string
	Username string
	Password string
}

type servicesLoadedMsg struct {
	services []discoveredService
	err      error
}

type watcherStartedMsg struct {
	name   string
	cmd    *exec.Cmd
	cancel context.CancelFunc
}
type watcherStoppedMsg struct {
	name string
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

func NewGeneralModel(envType, shopURL, username, password, projectRoot string, exec executor.Executor) GeneralModel {
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
		loading:     true,
	}
}

func (m GeneralModel) Init() tea.Cmd {
	return discoverServices(m.projectRoot)
}

func (m *GeneralModel) SetSize(width, height int) {
	m.width = width
	m.height = height
}

type browserOpenedMsg struct{}

func (m GeneralModel) Update(msg tea.Msg) (GeneralModel, tea.Cmd) {
	if msg, ok := msg.(servicesLoadedMsg); ok {
		m.loading = false
		m.services = msg.services
		m.err = msg.err
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
	s.WriteString(tui.KVRow("Environment", activeBadgeStyle.Render(m.envType)))
	s.WriteString(tui.KVRow("Shop URL", urlStyle.Render(m.shopURL)))
	s.WriteString(tui.KVRow("Admin URL", urlStyle.Render(m.adminURL)))

	s.WriteString(divider)

	s.WriteString(tui.TitleStyle.Render("Admin Access"))
	s.WriteString("\n")
	if m.username == "" && m.password == "" {
		s.WriteString("  " + helpStyle.Render("Credentials will appear here once Shopware is installed.") + "\n")
	} else {
		s.WriteString(tui.KVRow("Username", valueStyle.Render(m.username)))
		s.WriteString(tui.KVRow("Password", secretStyle.Render(m.password)))
	}

	s.WriteString(divider)

	s.WriteString(tui.TitleStyle.Render("Watchers"))
	s.WriteString("\n")
	s.WriteString(m.renderWatcherStatus("Admin", m.adminWatchRunning, m.adminWatchStarting, "http://127.0.0.1:5173"))
	s.WriteString(m.renderWatcherStatus("Storefront", m.sfWatchRunning, m.sfWatchStarting, ""))

	s.WriteString(divider)

	s.WriteString(tui.TitleStyle.Render("Services"))
	s.WriteString("\n")
	s.WriteString(m.renderServices())

	return s.String()
}

func (m *GeneralModel) startAdminWatch() tea.Cmd {
	e := m.executor
	projectRoot := m.projectRoot

	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())

		watchCmd, err := extension.PrepareAdminWatcher(ctx, projectRoot, e)
		if err != nil {
			cancel()
			return watcherStoppedMsg{name: watcherAdmin}
		}

		return watcherStartedMsg{name: watcherAdmin, cmd: watchCmd, cancel: cancel}
	}
}

func (m *GeneralModel) startStorefrontWatch() tea.Cmd {
	e := m.executor
	projectRoot := m.projectRoot

	return func() tea.Msg {
		ctx, cancel := context.WithCancel(context.Background())

		watchCmd, err := extension.PrepareStorefrontWatcher(ctx, projectRoot, e)
		if err != nil {
			cancel()
			return watcherStoppedMsg{name: watcherStorefront}
		}

		return watcherStartedMsg{name: watcherStorefront, cmd: watchCmd, cancel: cancel}
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

// dockerComposePSOutput represents a single container from `docker compose ps --format json`.
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

func discoverServices(projectRoot string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--format", "json")
		cmd.Dir = projectRoot
		output, err := cmd.Output()
		if err != nil {
			return servicesLoadedMsg{err: fmt.Errorf("docker compose ps: %w", err)}
		}

		var services []discoveredService

		// Collect all containers with their published ports
		type containerInfo struct {
			service    string
			publishers map[int]int // targetPort -> publishedPort
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

		// Match containers against known services or skip ignored ones
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

			services = append(services, discoveredService{
				Name:     known.Name,
				URL:      fmt.Sprintf("http://127.0.0.1:%d", publishedPort),
				Username: known.Username,
				Password: known.Password,
			})
		}

		return servicesLoadedMsg{services: services}
	}
}

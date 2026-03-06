package devtui

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type GeneralModel struct {
	envType     string
	shopURL     string
	adminURL    string
	username    string
	password    string
	services    []discoveredService
	projectRoot string
	loading     bool
	err         error
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

// knownService defines a well-known Docker compose service with its primary UI port and default credentials.
type knownService struct {
	Name       string
	TargetPort int
	Username   string
	Password   string
}

// knownServices maps compose service names to their known configuration.
// The key is the compose service name (e.g. "database", "mailer", "lavinmq").
var knownServices = map[string]knownService{
	"adminer":  {Name: "Adminer", TargetPort: 8080, Username: "root", Password: "root"},
	"mailer":   {Name: "Mailpit", TargetPort: 8025},
	"lavinmq":  {Name: "Queue (LavinMQ)", TargetPort: 15672, Username: "guest", Password: "guest"},
	"rabbitmq": {Name: "Queue (RabbitMQ)", TargetPort: 15672, Username: "guest", Password: "guest"},
}

// ignoredServices are compose services whose ports should not be listed.
var ignoredServices = map[string]bool{
	"web":      true,
	"database": true,
}

func NewGeneralModel(envType, shopURL, username, password, projectRoot string) GeneralModel {
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
		loading:     true,
	}
}

func (m GeneralModel) Init() tea.Cmd {
	return discoverServices(m.projectRoot)
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

func (m GeneralModel) View() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString(labelStyle.Render("Environment") + valueStyle.Render(m.envType) + "\n")
	b.WriteString(labelStyle.Render("Shop URL") + valueStyle.Render(m.shopURL) + "\n")
	b.WriteString(labelStyle.Render("Admin URL") + valueStyle.Render(m.adminURL) + "\n")

	if m.username != "" {
		b.WriteString(labelStyle.Render("Admin User") + valueStyle.Render(m.username) + "\n")
		b.WriteString(labelStyle.Render("Admin Password") + valueStyle.Render(m.password) + "\n")
	}

	b.WriteString("\n")

	switch {
	case m.loading:
		b.WriteString(helpStyle.Render("Discovering services...") + "\n")
	case m.err != nil:
		b.WriteString(errorStyle.Render("Service discovery failed: "+m.err.Error()) + "\n")
	case len(m.services) > 0:
		for _, s := range m.services {
			b.WriteString(labelStyle.Render(s.Name) + valueStyle.Render(s.URL) + "\n")
			if s.Username != "" {
				b.WriteString(labelStyle.Render("  Username") + valueStyle.Render(s.Username) + "\n")
				b.WriteString(labelStyle.Render("  Password") + valueStyle.Render(s.Password) + "\n")
			}
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("f: open shop | a: open admin"))

	return b.String()
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

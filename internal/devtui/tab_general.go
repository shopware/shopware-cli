package devtui

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
	width       int
	height      int
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

func (m GeneralModel) View() string {
	overviewRows := []string{
		renderKVRow("Environment", m.envType, activeBadgeStyle),
		renderKVRow("Shop URL", m.shopURL, urlStyle),
		renderKVRow("Admin URL", m.adminURL, urlStyle),
	}

	credentialsRows := []string{
		renderKVRow("Username", m.username, valueStyle),
		renderKVRow("Password", m.password, secretStyle),
	}

	if m.username == "" && m.password == "" {
		credentialsRows = []string{
			helpStyle.Render("Admin credentials will appear here once Shopware is installed."),
		}
	}

	contentWidth := max(m.width-2, 0)

	var content string
	if m.width >= 110 {
		columnWidth := contentWidth / 2
		overviewSection := renderSectionWidth("Shop", strings.Join(overviewRows, "\n"), columnWidth)
		credentialsSection := renderSectionWidth("Admin Access", strings.Join(credentialsRows, "\n"), columnWidth)
		servicesSection := renderSection("Services", m.renderServices())

		leftCol := padLines(overviewSection, columnWidth, surfaceTextStyle)
		rightCol := padLines(credentialsSection, columnWidth, surfaceTextStyle)
		leftLines := strings.Split(leftCol, "\n")
		rightLines := strings.Split(rightCol, "\n")
		rowHeight := max(len(leftLines), len(rightLines))
		emptyLine := surfaceTextStyle.Render(strings.Repeat(" ", columnWidth))
		for len(leftLines) < rowHeight {
			leftLines = append(leftLines, emptyLine)
		}
		for len(rightLines) < rowHeight {
			rightLines = append(rightLines, emptyLine)
		}
		var topRowLines []string
		for i := range rowHeight {
			topRowLines = append(topRowLines, leftLines[i]+rightLines[i])
		}
		topRow := strings.Join(topRowLines, "\n")
		content = topRow + "\n" + padLines(servicesSection, contentWidth, surfaceTextStyle)
	} else {
		overviewSection := renderSection("Shop", strings.Join(overviewRows, "\n"))
		credentialsSection := renderSection("Admin Access", strings.Join(credentialsRows, "\n"))
		servicesSection := renderSection("Services", m.renderServices())
		content = lipgloss.JoinVertical(
			lipgloss.Left,
			overviewSection,
			credentialsSection,
			servicesSection,
		)
	}

	content = padLines(content, contentWidth, surfaceTextStyle)

	var footerHints []string
	if m.shopURL != "" {
		footerHints = append(footerHints, renderKeyHint("f", "Open shop"))
	}
	if m.adminURL != "" && m.adminURL != "admin" {
		footerHints = append(footerHints, renderKeyHint("a", "Open admin"))
	}

	footer := padLines(renderFooter(footerHints...), contentWidth, surfaceTextStyle)

	body := "\n" + content

	// Pin footer to bottom by filling remaining height with styled whitespace
	bodyHeight := lipgloss.Height(body)
	footerHeight := lipgloss.Height(footer)
	if gap := m.height - bodyHeight - footerHeight; gap > 0 {
		emptyLine := surfaceTextStyle.Render(strings.Repeat(" ", contentWidth))
		for range gap {
			body += "\n" + emptyLine
		}
	}

	return body + "\n" + footer
}

func (m GeneralModel) renderServices() string {
	switch {
	case m.loading:
		return lipgloss.JoinVertical(
			lipgloss.Left,
			activeBadgeStyle.Render("SCANNING"),
			helpStyle.Render("Looking for published local services."),
		)
	case m.err != nil:
		return lipgloss.JoinVertical(
			lipgloss.Left,
			errorBadgeStyle.Render("DISCOVERY FAILED"),
			errorStyle.Render(m.err.Error()),
		)
	case len(m.services) == 0:
		return helpStyle.Render("No auxiliary services detected.")
	}

	blocks := make([]string, 0, len(m.services))
	for _, service := range m.services {
		rows := []string{
			renderKVRow(service.Name, service.URL, urlStyle),
		}
		if service.Username != "" {
			rows = append(rows, renderSubKVRow("Username", service.Username, valueStyle))
			rows = append(rows, renderSubKVRow("Password", service.Password, secretStyle))
		}

		blocks = append(blocks, strings.Join(rows, "\n"))
	}

	return strings.Join(blocks, "\n\n")
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

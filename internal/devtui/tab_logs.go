package devtui

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/shopware/shopware-cli/internal/tui"
)

type logSource struct {
	name      string
	container string // non-empty for docker containers
	filePath  string // non-empty for log files
}

type LogsModel struct {
	viewport    viewport.Model
	sources     []logSource
	cursor      int
	active      int // index of currently streaming source
	lines       []string
	follow      bool
	cancel      context.CancelFunc
	logChan     <-chan string
	projectRoot string
	dockerMode  bool
	width       int
	height      int
}

type logLineMsg string
type logDoneMsg struct{}
type logErrMsg struct{ err error }
type logSourcesLoadedMsg struct{ sources []logSource }

const sidebarWidth = 28

func NewLogsModel(projectRoot string, dockerMode bool) LogsModel {
	return LogsModel{
		projectRoot: projectRoot,
		dockerMode:  dockerMode,
		follow:      true,
		active:      -1,
	}
}

func (m LogsModel) Init() tea.Cmd {
	return nil
}

func (m LogsModel) Update(msg tea.Msg) (LogsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case logSourcesLoadedMsg:
		m.sources = msg.sources
		if len(m.sources) > 0 && m.active == -1 {
			m.active = 0
			m.cursor = 0
			return m, m.startCurrentSource()
		}
		return m, nil

	case logLineMsg:
		m.lines = append(m.lines, string(msg))
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, m.waitForNextLine()

	case logDoneMsg:
		m.lines = append(m.lines, helpStyle.Render("--- log stream ended ---"))
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		return m, nil

	case logErrMsg:
		m.lines = append(m.lines, errorStyle.Render("Log stream error: "+msg.err.Error()))
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case keyUp, keyK:
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case keyDown, keyJ:
			if m.cursor < len(m.sources)-1 {
				m.cursor++
			}
			return m, nil
		case keyEnter:
			if m.cursor != m.active && m.cursor < len(m.sources) {
				m.stopStreaming()
				m.active = m.cursor
				m.lines = nil
				m.follow = true
				m.viewport.SetContent("")
				m.viewport.GotoTop()
				return m, m.startCurrentSource()
			}
			return m, nil
		case keyF:
			m.follow = !m.follow
			if m.follow {
				m.viewport.GotoBottom()
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)

	if !m.viewport.AtBottom() {
		m.follow = false
	}

	return m, cmd
}

func (m LogsModel) View() string {
	return lipgloss.JoinHorizontal(lipgloss.Top, m.renderSidebar(), m.renderContent())
}

func (m LogsModel) renderSidebar() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Sources"))
	b.WriteString("\n\n")

	for i, src := range m.sources {
		item := src.name
		if i == m.active {
			item = lipgloss.JoinHorizontal(
				lipgloss.Center,
				item,
				" ",
				activeBadgeStyle.Render("LIVE"),
			)
		}

		style := sidebarItemStyle
		switch {
		case i == m.cursor && m.cursor == m.active:
			style = activeSelectedSidebarItemStyle
		case i == m.cursor:
			style = selectedSidebarItemStyle
		case i == m.active:
			style = activeSidebarItemStyle
		}

		b.WriteString(style.Width(sidebarWidth - 4).Render(item))
		b.WriteString("\n")
	}

	if len(m.sources) == 0 {
		b.WriteString(helpStyle.Render("No log sources found yet."))
	}

	return sidebarStyle.
		Width(sidebarWidth).
		Height(max(m.height-3, 8)).
		Render(b.String())
}

func (m LogsModel) renderContent() string {
	sourceName := "No source selected"
	if m.active >= 0 && m.active < len(m.sources) {
		sourceName = m.sources[m.active].name
	}

	followBadge := warningBadgeStyle.Render("FOLLOW OFF")
	if m.follow {
		followBadge = activeBadgeStyle.Render("FOLLOW ON")
	}

	headerText := lipgloss.NewStyle().
		Foreground(tui.TextColor).
		Bold(true).
		Render(sourceName)

	header := headerText + " " + followBadge

	return contentPanelStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			"",
			m.viewport.View(),
		),
	)
}

func (m *LogsModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	viewportWidth := max(width-sidebarWidth-8, 20)
	m.viewport.SetWidth(viewportWidth)
	m.viewport.SetHeight(max(height-7, 8))
}

func (m *LogsModel) StartStreaming() tea.Cmd {
	return m.discoverSources()
}

func (m *LogsModel) StopStreaming() {
	m.stopStreaming()
}

func (m *LogsModel) stopStreaming() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.logChan = nil
}

func (m *LogsModel) startCurrentSource() tea.Cmd {
	if m.active < 0 || m.active >= len(m.sources) {
		return nil
	}

	src := m.sources[m.active]

	if src.container != "" {
		return m.streamContainer(src.container)
	}

	return m.streamFile(src.filePath)
}

func (m *LogsModel) streamContainer(container string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	ch := make(chan string, 100)
	m.logChan = ch

	go func() {
		defer close(ch)

		cmd := exec.CommandContext(ctx, "docker", "compose", "logs", "-f", "--tail=100", container)
		cmd.Dir = m.projectRoot

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return
		}
		cmd.Stderr = cmd.Stdout

		if err := cmd.Start(); err != nil {
			return
		}

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case ch <- scanner.Text():
			}
		}

		_ = cmd.Wait()
	}()

	return m.waitForNextLine()
}

func (m *LogsModel) streamFile(filePath string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	ch := make(chan string, 100)
	m.logChan = ch

	go func() {
		defer close(ch)

		cmd := exec.CommandContext(ctx, "tail", "-n", "100", "-f", filePath)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return
		}

		if err := cmd.Start(); err != nil {
			return
		}

		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case ch <- scanner.Text():
			}
		}

		_ = cmd.Wait()
	}()

	return m.waitForNextLine()
}

func (m *LogsModel) waitForNextLine() tea.Cmd {
	ch := m.logChan
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return logDoneMsg{}
		}
		return logLineMsg(line)
	}
}

func (m *LogsModel) discoverSources() tea.Cmd {
	projectRoot := m.projectRoot
	dockerMode := m.dockerMode
	return func() tea.Msg {
		var sources []logSource

		if dockerMode {
			sources = append(sources, discoverContainers(projectRoot)...)
		}

		sources = append(sources, discoverLogFiles(projectRoot)...)

		return logSourcesLoadedMsg{sources: sources}
	}
}

func discoverContainers(projectRoot string) []logSource {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "docker", "compose", "ps", "--format", "{{.Service}}")
	cmd.Dir = projectRoot
	output, err := cmd.Output()
	if err != nil {
		return nil
	}

	var sources []logSource
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sources = append(sources, logSource{
			name:      line,
			container: line,
		})
	}
	return sources
}

func discoverLogFiles(projectRoot string) []logSource {
	logDir := filepath.Join(projectRoot, "var", "log")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil
	}

	var sources []logSource
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		sources = append(sources, logSource{
			name:     entry.Name(),
			filePath: filepath.Join(logDir, entry.Name()),
		})
	}
	return sources
}

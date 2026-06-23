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

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/tui"
)

type logSourceKind int

const (
	sourceContainer logSourceKind = iota // docker compose logs <service>
	sourceProcess                        // watcher / app process (lineChan or process)
	sourceFile                           // var/log/*.log
)

type logSource struct {
	name      string
	kind      logSourceKind
	container string
	filePath  string
	process   *executor.Process
	lineChan  <-chan string
	dead      bool
}

type InstanceModel struct {
	viewport      viewport.Model
	sources       []logSource
	cursor        int
	active        int
	lines         []string
	follow        bool
	cancel        context.CancelFunc
	activeProcess *executor.Process
	logChan       <-chan string
	projectRoot   string
	dockerMode    bool
	width         int
	height        int
}

type logLineMsg string
type logDoneMsg struct{ source string }
type logErrMsg struct{ err error }
type logSourcesLoadedMsg struct{ sources []logSource }

const sidebarWidth = 28

func NewInstanceModel(projectRoot string, dockerMode bool) InstanceModel {
	return InstanceModel{
		projectRoot: projectRoot,
		dockerMode:  dockerMode,
		follow:      true,
		active:      -1,
	}
}

func (m InstanceModel) Init() tea.Cmd {
	return nil
}

func (m InstanceModel) Update(msg tea.Msg) (InstanceModel, tea.Cmd) {
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
		if m.active >= 0 && m.active < len(m.sources) {
			src := m.sources[m.active]
			if src.process != nil || src.lineChan != nil {
				m.sources[m.active].dead = true
			}
		}
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

func (m InstanceModel) View() string {
	return lipgloss.JoinHorizontal(lipgloss.Top, m.renderSidebar(), m.renderContent())
}

func (m InstanceModel) renderSidebar() string {
	var b strings.Builder
	b.WriteString(tui.TitleStyle.Render("Instance Stack"))
	b.WriteString("\n")

	if len(m.sources) == 0 {
		b.WriteString(helpStyle.Render("No sources found yet."))
		return sidebarStyle.
			Width(sidebarWidth).
			Height(max(m.height-3, 8)).
			Render(b.String())
	}

	// Group sources by kind in a fixed order: containers, processes, files.
	type group struct {
		header  string
		sources []int // indices into m.sources
	}
	groupOrder := []logSourceKind{sourceContainer, sourceProcess, sourceFile}
	groupHeaders := map[logSourceKind]string{
		sourceContainer: "Containers",
		sourceProcess:   "Processes",
		sourceFile:      "Log Files",
	}
	var groups []group
	for _, kind := range groupOrder {
		var indices []int
		for i, src := range m.sources {
			if src.kind == kind {
				indices = append(indices, i)
			}
		}
		if len(indices) > 0 {
			groups = append(groups, group{header: groupHeaders[kind], sources: indices})
		}
	}

	groupHeaderStyle := lipgloss.NewStyle().
		Foreground(tui.MutedColor).
		Bold(true).
		Padding(0, 1)

	for gi, g := range groups {
		if gi > 0 {
			b.WriteString("\n")
		}
		b.WriteString(groupHeaderStyle.Render(strings.ToUpper(g.header)))
		b.WriteString("\n")

		for _, i := range g.sources {
			src := m.sources[i]
			indicator := tui.DimStyle.Render("·")
			if i == m.active {
				indicator = activeBadgeStyle.Render("●")
			} else if src.kind == sourceProcess && (src.lineChan != nil || src.process != nil) && !src.dead {
				indicator = activeBadgeStyle.Render("●")
			}

			item := lipgloss.JoinHorizontal(lipgloss.Center, indicator, " ", src.name)
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

			if i == m.cursor {
				item = lipgloss.JoinHorizontal(lipgloss.Center, lipgloss.NewStyle().Foreground(tui.BrandColor).Bold(true).Render("▸"), " ", item)
			} else {
				item = lipgloss.JoinHorizontal(lipgloss.Center, "  ", item)
			}

			b.WriteString(style.Width(sidebarWidth - 4).Render(item))
			b.WriteString("\n")
		}
	}

	return sidebarStyle.
		Width(sidebarWidth).
		Height(max(m.height-3, 8)).
		Render(b.String())
}

func (m InstanceModel) renderContent() string {
	sourceName := "No source selected"
	kindLabel := ""
	if m.active >= 0 && m.active < len(m.sources) {
		src := m.sources[m.active]
		sourceName = src.name
		switch src.kind {
		case sourceContainer:
			kindLabel = "container"
		case sourceProcess:
			kindLabel = "process"
		case sourceFile:
			kindLabel = "file"
		}
	}

	followBadge := warningBadgeStyle.Render("FOLLOW OFF")
	if m.follow {
		followBadge = activeBadgeStyle.Render("FOLLOW ON")
	}

	headerText := lipgloss.NewStyle().
		Foreground(tui.TextColor).
		Bold(true).
		Render(sourceName)

	header := headerText
	if kindLabel != "" {
		header += " " + tui.TextBadge(kindLabel)
	}
	header += " " + followBadge

	return contentPanelStyle.Render(
		lipgloss.JoinVertical(
			lipgloss.Left,
			header,
			"",
			m.viewport.View(),
		),
	)
}

func (m *InstanceModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	viewportWidth := max(width-sidebarWidth-8, 20)
	m.viewport.SetWidth(viewportWidth)
	m.viewport.SetHeight(max(height-7, 8))
}

func (m *InstanceModel) StartStreaming() tea.Cmd {
	return m.discoverSources()
}

func (m *InstanceModel) AddProcessSource(name string, process *executor.Process) tea.Cmd {
	m.stopStreaming()

	src := logSource{name: name, kind: sourceProcess, process: process}

	idx := -1
	for i, s := range m.sources {
		if s.name == name {
			m.sources[i] = src
			idx = i
			break
		}
	}
	if idx == -1 {
		m.sources = append(m.sources, src)
		idx = len(m.sources) - 1
	}

	m.active = idx
	m.cursor = idx
	m.lines = nil
	m.follow = true
	m.viewport.SetContent("")
	m.viewport.GotoTop()

	return m.streamProcess(src)
}

// AddStreamingSource registers a log source backed by an externally fed line
// channel (e.g. a watcher's preparation steps) and starts displaying it live.
func (m *InstanceModel) AddStreamingSource(name string, lineChan <-chan string) tea.Cmd {
	m.stopStreaming()

	src := logSource{name: name, kind: sourceProcess, lineChan: lineChan}

	idx := -1
	for i, s := range m.sources {
		if s.name == name {
			m.sources[i] = src
			idx = i
			break
		}
	}
	if idx == -1 {
		m.sources = append(m.sources, src)
		idx = len(m.sources) - 1
	}

	m.active = idx
	m.cursor = idx
	m.lines = nil
	m.follow = true
	m.viewport.SetContent("")
	m.viewport.GotoTop()

	m.logChan = lineChan
	return m.waitForNextLine()
}

func (m *InstanceModel) StopStreaming() {
	m.stopStreaming()
}

func (m *InstanceModel) AppendErrorLine(msg string) {
	m.lines = append(m.lines, errorStyle.Render(msg))
	m.viewport.SetContent(strings.Join(m.lines, "\n"))
	if m.follow {
		m.viewport.GotoBottom()
	}
}

func (m *InstanceModel) ActiveProcessSourceName() string {
	if m.active >= 0 && m.active < len(m.sources) {
		src := m.sources[m.active]
		if src.process != nil || src.lineChan != nil {
			return src.name
		}
	}
	return ""
}

func (m *InstanceModel) stopStreaming() {
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.activeProcess = nil
	m.logChan = nil
}

func (m *InstanceModel) startCurrentSource() tea.Cmd {
	if m.active < 0 || m.active >= len(m.sources) {
		return nil
	}

	src := m.sources[m.active]

	if src.lineChan != nil {
		if src.dead {
			return nil
		}
		m.logChan = src.lineChan
		return m.waitForNextLine()
	}

	if src.process != nil {
		if src.dead {
			return nil
		}
		return m.streamProcess(src)
	}

	if src.container != "" {
		return m.streamContainer(src.container)
	}

	return m.streamFile(src.filePath)
}

func (m *InstanceModel) streamProcess(src logSource) tea.Cmd {
	p := src.process
	cmd := p.Cmd
	m.activeProcess = p

	ch := make(chan string, 100)
	m.logChan = ch

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		close(ch)
		return m.waitForNextLine()
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		close(ch)
		return m.waitForNextLine()
	}

	go func() {
		defer close(ch)
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			ch <- scanner.Text()
		}
		_ = scanner.Err()
		_ = cmd.Wait()
	}()

	return m.waitForNextLine()
}

func (m *InstanceModel) streamContainer(container string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	cmd := exec.CommandContext(ctx, "docker", "compose", "logs", "-f", "--tail=100", container)
	cmd.Dir = m.projectRoot

	return m.streamCommand(ctx, cmd, true)
}

func (m *InstanceModel) streamFile(filePath string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel

	cmd := exec.CommandContext(ctx, "tail", "-n", "100", "-f", filePath)

	return m.streamCommand(ctx, cmd, false)
}

func (m *InstanceModel) streamCommand(ctx context.Context, cmd *exec.Cmd, mergeStderr bool) tea.Cmd {
	ch := make(chan string, 100)
	m.logChan = ch

	go func() {
		defer close(ch)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return
		}
		if mergeStderr {
			cmd.Stderr = cmd.Stdout
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
		_ = scanner.Err()

		_ = cmd.Wait()
	}()

	return m.waitForNextLine()
}

func (m *InstanceModel) waitForNextLine() tea.Cmd {
	ch := m.logChan
	if ch == nil {
		return nil
	}
	source := m.ActiveProcessSourceName()
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return logDoneMsg{source: source}
		}
		return logLineMsg(line)
	}
}

func (m *InstanceModel) discoverSources() tea.Cmd {
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
			kind:      sourceContainer,
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
			kind:     sourceFile,
			filePath: filepath.Join(logDir, entry.Name()),
		})
	}
	return sources
}

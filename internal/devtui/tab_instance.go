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
	// lines holds this source's accumulated scrollback so switching between
	// sources and back does not lose previously streamed output.
	lines []string
}

type InstanceModel struct {
	viewport    viewport.Model
	sources     []logSource
	cursor      int
	active      int
	follow      bool
	projectRoot string
	dockerMode  bool
	width       int
	height      int

	// linesChan is the shared fan-in channel every source's stream writes to.
	// Each streamed line is tagged with the source it belongs to so background
	// sources keep accumulating scrollback while the user views another one.
	linesChan chan taggedLine
	// cancels holds the per-source cancel funcs so all streams can be torn down
	// on shutdown. Keyed by source name (stable across re-discovery).
	cancels map[string]context.CancelFunc
	// streaming tracks which source names already have a live stream so we don't
	// start a second one when re-discovered or re-selected.
	streaming map[string]bool
	// reading is true once the single fan-in reader command is armed, so we
	// never run two concurrent readers on linesChan (which would split lines).
	reading bool
}

// taggedLine is a single log line annotated with the name of the source it came
// from, used for fan-in routing into the correct per-source buffer.
type taggedLine struct {
	source string
	line   string
	done   bool
}

type logLineMsg taggedLine
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
		linesChan:   make(chan taggedLine, 256),
		cancels:     make(map[string]context.CancelFunc),
		streaming:   make(map[string]bool),
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
		}
		// Start every source streaming in the background so switching between
		// them never loses scrollback. Reading the shared fan-in channel begins
		// as soon as the first stream is armed.
		if len(m.sources) == 0 {
			return m, nil
		}
		cmds := m.startAllSources()
		cmds = append(cmds, m.ensureReading())
		return m, tea.Batch(cmds...)

	case logLineMsg:
		i := m.indexOfSource(msg.source)
		if i < 0 {
			return m, m.readNextLine()
		}
		m.sources[i].lines = append(m.sources[i].lines, msg.line)
		if i == m.active {
			m.refreshViewport()
		}
		return m, m.readNextLine()

	case logDoneMsg:
		i := m.indexOfSource(msg.source)
		if i >= 0 {
			m.sources[i].lines = append(m.sources[i].lines, helpStyle.Render("--- log stream ended ---"))
			src := m.sources[i]
			if src.process != nil || src.lineChan != nil {
				m.sources[i].dead = true
			}
			delete(m.streaming, msg.source)
			if i == m.active {
				m.refreshViewport()
			}
		}
		return m, m.readNextLine()

	case logErrMsg:
		if m.active >= 0 && m.active < len(m.sources) {
			m.sources[m.active].lines = append(m.sources[m.active].lines, errorStyle.Render("Log stream error: "+msg.err.Error()))
			m.refreshViewport()
		}
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case keyUp, keyK:
			m.cursor = m.neighborCursor(-1)
			return m, nil
		case keyDown, keyJ:
			m.cursor = m.neighborCursor(1)
			return m, nil
		case keyEnter:
			if m.cursor != m.active && m.cursor < len(m.sources) {
				return m, m.switchTo(m.cursor)
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

type sidebarGroup struct {
	header  string
	sources []int // indices into m.sources
}

// groupedSources returns the sources grouped by kind in the fixed visual order
// (containers, processes, files). This is the order shown in the sidebar and the
// order arrow-key navigation must follow — m.sources itself is in discovery
// order, which interleaves processes added at runtime after files.
func (m InstanceModel) groupedSources() []sidebarGroup {
	groupOrder := []logSourceKind{sourceContainer, sourceProcess, sourceFile}
	groupHeaders := map[logSourceKind]string{
		sourceContainer: "Containers",
		sourceProcess:   "Processes",
		sourceFile:      "Log Files",
	}
	var groups []sidebarGroup
	for _, kind := range groupOrder {
		var indices []int
		for i, src := range m.sources {
			if src.kind == kind {
				indices = append(indices, i)
			}
		}
		if len(indices) > 0 {
			groups = append(groups, sidebarGroup{header: groupHeaders[kind], sources: indices})
		}
	}
	return groups
}

// visualOrder flattens the grouped sources into the list of m.sources indices in
// the order they appear on screen, top to bottom.
func (m InstanceModel) visualOrder() []int {
	var order []int
	for _, g := range m.groupedSources() {
		order = append(order, g.sources...)
	}
	return order
}

// neighborCursor returns the source index that is `delta` steps away from the
// current cursor in visual (on-screen) order, clamped to the ends.
func (m InstanceModel) neighborCursor(delta int) int {
	order := m.visualOrder()
	if len(order) == 0 {
		return m.cursor
	}

	pos := 0
	for i, idx := range order {
		if idx == m.cursor {
			pos = i
			break
		}
	}

	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos > len(order)-1 {
		pos = len(order) - 1
	}
	return order[pos]
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

	groups := m.groupedSources()

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
			switch {
			case i == m.active:
				// Solid brand dot: the source currently being viewed.
				indicator = brandColor.Render("●")
			case src.kind == sourceProcess && (src.lineChan != nil || src.process != nil) && !src.dead:
				// Hollow brand dot: a running process that is not the active source.
				indicator = brandColor.Render("◦")
			}

			item := lipgloss.JoinHorizontal(lipgloss.Center, indicator, " ", src.name)
			if i == m.active {
				item = lipgloss.JoinHorizontal(
					lipgloss.Center,
					item,
					" ",
					activeBadgeStyle.Render("FOLLOWING"),
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

	headerText := lipgloss.NewStyle().
		Foreground(tui.TextColor).
		Bold(true).
		Render(sourceName)

	header := headerText
	if kindLabel != "" {
		header += " " + tui.TextBadge(kindLabel)
	}

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
	return m.addSource(logSource{name: name, kind: sourceProcess, process: process})
}

// AddStreamingSource registers a log source backed by an externally fed line
// channel (e.g. a watcher's preparation steps) and starts displaying it live.
func (m *InstanceModel) AddStreamingSource(name string, lineChan <-chan string) tea.Cmd {
	return m.addSource(logSource{name: name, kind: sourceProcess, lineChan: lineChan})
}

// addSource inserts or replaces a source by name, focuses it, and starts its
// background stream. Existing sources keep streaming - only the new one is
// added, so no scrollback is lost.
func (m *InstanceModel) addSource(src logSource) tea.Cmd {
	idx := m.indexOfSource(src.name)
	if idx >= 0 {
		// Replacing an existing source (e.g. a watcher restarted): drop its old
		// stream so a fresh one can be armed for the new process/channel.
		if cancel := m.cancels[src.name]; cancel != nil {
			cancel()
			delete(m.cancels, src.name)
		}
		delete(m.streaming, src.name)
		src.lines = m.sources[idx].lines
		m.sources[idx] = src
	} else {
		m.sources = append(m.sources, src)
		idx = len(m.sources) - 1
	}

	m.active = idx
	m.cursor = idx
	m.follow = true
	m.refreshViewport()

	var cmds []tea.Cmd
	if cmd := m.startSource(idx); cmd != nil {
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, m.ensureReading())
	return tea.Batch(cmds...)
}

func (m *InstanceModel) StopStreaming() {
	m.stopStreaming()
}

// RemoveSource stops the named source's stream and drops it from the sidebar,
// e.g. when a watcher is stopped. The active/cursor selection is kept valid and
// moved to a neighbouring source, re-rendering its buffer if the removed source
// was the active one.
func (m *InstanceModel) RemoveSource(name string) {
	idx := m.indexOfSource(name)
	if idx < 0 {
		return
	}

	if cancel := m.cancels[name]; cancel != nil {
		cancel()
		delete(m.cancels, name)
	}
	delete(m.streaming, name)

	m.sources = append(m.sources[:idx], m.sources[idx+1:]...)

	if len(m.sources) == 0 {
		m.active = -1
		m.cursor = 0
		m.refreshViewport()
		return
	}

	// Keep active/cursor in range; shift them left when they sat at or after the
	// removed index so they keep pointing at the same visible rows.
	activeRemoved := m.active == idx
	if m.active > idx || m.active >= len(m.sources) {
		m.active--
	}
	if m.cursor > idx || m.cursor >= len(m.sources) {
		m.cursor--
	}
	if m.active < 0 {
		m.active = 0
	}
	if m.cursor < 0 {
		m.cursor = 0
	}

	if activeRemoved {
		m.follow = true
		m.refreshViewport()
	}
}

func (m *InstanceModel) AppendErrorLine(msg string) {
	if m.active < 0 || m.active >= len(m.sources) {
		return
	}
	m.sources[m.active].lines = append(m.sources[m.active].lines, errorStyle.Render(msg))
	m.refreshViewport()
}

func (m *InstanceModel) stopStreaming() {
	for name, cancel := range m.cancels {
		cancel()
		delete(m.cancels, name)
	}
	m.streaming = map[string]bool{}
}

// indexOfSource returns the index of the source with the given name, or -1.
func (m *InstanceModel) indexOfSource(name string) int {
	for i, s := range m.sources {
		if s.name == name {
			return i
		}
	}
	return -1
}

// refreshViewport re-renders the active source's buffer into the viewport,
// keeping the view pinned to the bottom while following.
func (m *InstanceModel) refreshViewport() {
	if m.active < 0 || m.active >= len(m.sources) {
		m.viewport.SetContent("")
		return
	}
	m.viewport.SetContent(strings.Join(m.sources[m.active].lines, "\n"))
	if m.follow {
		m.viewport.GotoBottom()
	}
}

// switchTo makes source idx the active one. All sources stream in the
// background, so switching only swaps which buffer is displayed - no scrollback
// is lost and streams are neither stopped nor restarted.
func (m *InstanceModel) switchTo(idx int) tea.Cmd {
	m.active = idx
	m.cursor = idx
	m.follow = true
	m.refreshViewport()

	// A freshly selected source might not have been streaming yet (e.g. a source
	// discovered after the initial batch); ensure it is.
	cmds := m.startAllSources()
	cmds = append(cmds, m.ensureReading())
	return tea.Batch(cmds...)
}

// startAllSources arms a background stream for every source that is not already
// streaming and can stream. Returns the commands that begin each stream.
func (m *InstanceModel) startAllSources() []tea.Cmd {
	var cmds []tea.Cmd
	for i := range m.sources {
		if cmd := m.startSource(i); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return cmds
}

// startSource arms a background stream for a single source if it is not already
// streaming and is not a dead process.
func (m *InstanceModel) startSource(i int) tea.Cmd {
	src := m.sources[i]
	if m.streaming[src.name] || src.dead {
		return nil
	}

	switch {
	case src.lineChan != nil:
		m.streaming[src.name] = true
		return m.pumpChan(src.name, src.lineChan)
	case src.process != nil:
		return m.streamProcess(src)
	case src.container != "":
		return m.streamContainer(src.name, src.container)
	case src.filePath != "":
		return m.streamFile(src.name, src.filePath)
	}
	return nil
}

// pumpChan forwards an externally-fed line channel into the shared fan-in
// channel, tagging each line with its source name.
func (m *InstanceModel) pumpChan(name string, in <-chan string) tea.Cmd {
	out := m.linesChan
	go func() {
		for line := range in {
			out <- taggedLine{source: name, line: line}
		}
		out <- taggedLine{source: name, done: true}
	}()
	return nil
}

func (m *InstanceModel) streamProcess(src logSource) tea.Cmd {
	name := src.name
	cmd := src.process.Cmd
	out := m.linesChan

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return func() tea.Msg { return logDoneMsg{source: name} }
	}
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return func() tea.Msg { return logDoneMsg{source: name} }
	}

	m.streaming[name] = true
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			out <- taggedLine{source: name, line: scanner.Text()}
		}
		_ = scanner.Err()
		_ = cmd.Wait()
		out <- taggedLine{source: name, done: true}
	}()

	return nil
}

func (m *InstanceModel) streamContainer(name, container string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[name] = cancel
	cmd := composeCommand(ctx, m.projectRoot, "logs", "-f", "--tail=100", container)
	return m.streamCommand(ctx, name, cmd, true)
}

func (m *InstanceModel) streamFile(name, filePath string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancels[name] = cancel
	cmd := exec.CommandContext(ctx, "tail", "-n", "100", "-f", filePath)
	return m.streamCommand(ctx, name, cmd, false)
}

func (m *InstanceModel) streamCommand(ctx context.Context, name string, cmd *exec.Cmd, mergeStderr bool) tea.Cmd {
	out := m.linesChan
	m.streaming[name] = true

	go func() {
		defer func() { out <- taggedLine{source: name, done: true} }()

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
			case out <- taggedLine{source: name, line: scanner.Text()}:
			}
		}
		_ = scanner.Err()
		_ = cmd.Wait()
	}()

	return nil
}

// ensureReading arms the single fan-in reader if it is not already running.
// The reader re-arms itself on every message, so it only needs starting once.
func (m *InstanceModel) ensureReading() tea.Cmd {
	if m.reading {
		return nil
	}
	m.reading = true
	return m.readNextLine()
}

// readNextLine blocks on the shared fan-in channel and turns the next tagged
// line into the appropriate message.
func (m *InstanceModel) readNextLine() tea.Cmd {
	ch := m.linesChan
	return func() tea.Msg {
		tl := <-ch
		if tl.done {
			return logDoneMsg{source: tl.source}
		}
		return logLineMsg(tl)
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
	cmd := composeCommand(context.Background(), projectRoot, "ps", "--format", "{{.Service}}")
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

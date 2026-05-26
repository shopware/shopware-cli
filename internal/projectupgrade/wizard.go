package projectupgrade

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/shyim/go-version"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/flexmigrator"
	"github.com/shopware/shopware-cli/internal/tui"
)

// streamBufferSize is the buffered channel size used when streaming
// subprocess output to the wizard.
const streamBufferSize = 50

// maxLogLines caps how many output lines we keep in the running phase so the
// view doesn't grow unbounded.
const maxLogLines = 18

// WizardOptions configures a single run of the upgrade wizard.
type WizardOptions struct {
	ProjectRoot      string
	ComposerJSONPath string
	CurrentVersion   *version.Version
	UpdateVersions   []string
	Extensions       map[string]string
	Executor         executor.Executor
	// Registry is consulted to find newer compatible versions of plugins
	// whose installed shopware/core constraint is no longer satisfied. May
	// be nil, in which case incompatible plugins are simply removed.
	Registry Registry
}

type phase int

const (
	phaseWelcome phase = iota
	phaseSelectVersion
	phaseCompatCheck
	phaseCompatResult
	phaseReview
	phaseRunning
	phaseDone
)

type taskStatus int

const (
	taskPending taskStatus = iota
	taskRunning
	taskDone
	taskFailed
	taskSkipped
)

// task tracks one of the upgrade stages displayed in the running phase.
type task struct {
	label  string
	status taskStatus
	detail string
}

// taskCleanup, taskPlugins, ... are stable indices into model.tasks.
const (
	taskBackup = iota
	taskCleanup
	taskPlugins
	taskComposerJSON
	taskComposerUpdate
	taskSystemPrepare
	taskSystemFinish
)

// wizardMsg variants advance the upgrade state machine.
type (
	compatLoadedMsg struct {
		updates []account_api.UpdateCheckExtensionCompatibility
		err     error
	}
	taskCompleteMsg struct {
		task           int
		err            error
		detail         string
		composerBackup []byte
		pluginActions  *ResolveResult
	}
	startNextTaskMsg struct{}
	logLineMsg       string
	logDoneMsg       struct{}
)

// wizardModel is a small standalone bubbletea Program that walks the user
// through the Shopware upgrade in the same visual idiom as devtui's setup
// guide and install wizard.
type wizardModel struct {
	opts WizardOptions

	phase phase

	versionCursor   int
	targetVersion   string
	confirmYes      bool
	composerBackup  []byte
	pluginActions   *ResolveResult
	compatUpdates   []account_api.UpdateCheckExtensionCompatibility
	compatHasBlock  bool
	compatErr       error
	tasks           []task
	currentTask     int
	logLines        []string
	logChan         chan string
	finalErr        error
	finished        bool
	spinner         spinner.Model
	compatLoading   bool
	cancelExecution context.CancelFunc
}

// RunWizard runs the interactive upgrade wizard. It returns the selected
// target version, whether the upgrade completed successfully, and any error
// encountered. A user cancellation returns ErrCancelled.
func RunWizard(opts WizardOptions) (string, bool, error) {
	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(tui.BrandColor)),
	)

	m := wizardModel{
		opts:          opts,
		phase:         phaseWelcome,
		confirmYes:    true,
		versionCursor: 0,
		spinner:       s,
		tasks:         defaultTasks(),
	}

	prog := tea.NewProgram(m)
	final, err := prog.Run()
	if err != nil {
		return "", false, err
	}

	fm, _ := final.(wizardModel)
	if fm.cancelExecution != nil {
		fm.cancelExecution()
	}

	if !fm.finished {
		return fm.targetVersion, false, ErrCancelled
	}

	return fm.targetVersion, fm.finalErr == nil, fm.finalErr
}

// ErrCancelled is returned when the user exits the wizard before the upgrade
// completes (e.g. via q / ctrl+c or selecting the cancel button).
var ErrCancelled = errors.New("upgrade cancelled by user")

func defaultTasks() []task {
	return []task{
		{label: "Back up composer.json"},
		{label: "Clean up stale recipe files"},
		{label: "Resolve incompatible custom plugins"},
		{label: "Rewrite composer.json"},
		{label: "composer update --with-all-dependencies"},
		{label: "bin/console system:update:prepare"},
		{label: "bin/console system:update:finish"},
	}
}

func (m wizardModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.updateKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case compatLoadedMsg:
		m.compatLoading = false
		m.compatErr = msg.err
		m.compatUpdates = msg.updates
		for _, u := range msg.updates {
			if u.Status.IsBlocker() {
				m.compatHasBlock = true
				break
			}
		}
		m.phase = phaseCompatResult
		m.confirmYes = !m.compatHasBlock
		return m, nil

	case startNextTaskMsg:
		return m.startTask()

	case taskCompleteMsg:
		if msg.composerBackup != nil {
			m.composerBackup = msg.composerBackup
		}
		if msg.pluginActions != nil {
			m.pluginActions = msg.pluginActions
		}
		if msg.task < len(m.tasks) {
			if msg.err != nil {
				m.tasks[msg.task].status = taskFailed
				m.tasks[msg.task].detail = msg.err.Error()
			} else {
				m.tasks[msg.task].status = taskDone
				if msg.detail != "" {
					m.tasks[msg.task].detail = msg.detail
				}
			}
		}
		if msg.err != nil {
			m.finalErr = msg.err
			m.finished = true
			m.phase = phaseDone
			return m, nil
		}
		m.currentTask++
		if m.currentTask >= len(m.tasks) {
			m.finished = true
			m.phase = phaseDone
			return m, nil
		}
		return m, func() tea.Msg { return startNextTaskMsg{} }

	case logLineMsg:
		m.appendLog(string(msg))
		return m, m.readNextLog()

	case logDoneMsg:
		m.logChan = nil
		return m, nil
	}

	return m, nil
}

func (m wizardModel) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		if m.cancelExecution != nil {
			m.cancelExecution()
		}
		return m, tea.Quit
	}

	switch m.phase {
	case phaseWelcome:
		return m.updateWelcome(key)
	case phaseSelectVersion:
		return m.updateSelectVersion(key)
	case phaseCompatCheck:
		return m, nil
	case phaseCompatResult:
		return m.updateCompatResult(key)
	case phaseReview:
		return m.updateReview(key)
	case phaseRunning:
		return m, nil
	case phaseDone:
		if key == "q" || key == "enter" || key == "esc" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m wizardModel) updateWelcome(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		m.confirmYes = true
	case "right", "l":
		m.confirmYes = false
	case "tab":
		m.confirmYes = !m.confirmYes
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		if !m.confirmYes {
			return m, tea.Quit
		}
		m.phase = phaseSelectVersion
		return m, nil
	}
	return m, nil
}

func (m wizardModel) updateSelectVersion(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.versionCursor > 0 {
			m.versionCursor--
		}
	case "down", "j":
		if m.versionCursor < len(m.opts.UpdateVersions)-1 {
			m.versionCursor++
		}
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		m.targetVersion = m.opts.UpdateVersions[m.versionCursor]
		if len(m.opts.Extensions) == 0 {
			m.phase = phaseReview
			m.confirmYes = true
			return m, nil
		}
		m.phase = phaseCompatCheck
		m.compatLoading = true
		return m, tea.Batch(m.spinner.Tick, m.loadCompatibility())
	}
	return m, nil
}

func (m wizardModel) updateCompatResult(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		m.confirmYes = true
	case "right", "l":
		m.confirmYes = false
	case "tab":
		m.confirmYes = !m.confirmYes
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		if !m.confirmYes {
			return m, tea.Quit
		}
		m.phase = phaseReview
		m.confirmYes = true
		return m, nil
	}
	return m, nil
}

func (m wizardModel) updateReview(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		m.confirmYes = true
	case "right", "l":
		m.confirmYes = false
	case "tab":
		m.confirmYes = !m.confirmYes
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		if !m.confirmYes {
			return m, tea.Quit
		}
		m.phase = phaseRunning
		m.currentTask = 0
		return m, func() tea.Msg { return startNextTaskMsg{} }
	}
	return m, nil
}

func (m *wizardModel) appendLog(line string) {
	m.logLines = append(m.logLines, line)
	if len(m.logLines) > maxLogLines {
		m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
	}
}

func (m wizardModel) readNextLog() tea.Cmd {
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

// loadCompatibility queries the Shopware account API for extension
// compatibility against the chosen target version.
func (m wizardModel) loadCompatibility() tea.Cmd {
	requests := make([]account_api.UpdateCheckExtension, 0, len(m.opts.Extensions))
	for name, v := range m.opts.Extensions {
		requests = append(requests, account_api.UpdateCheckExtension{Name: name, Version: v})
	}
	currentVersion := m.opts.CurrentVersion.String()
	targetVersion := m.targetVersion

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		updates, err := account_api.GetFutureExtensionUpdates(ctx, currentVersion, targetVersion, requests)
		if err != nil {
			return compatLoadedMsg{err: err}
		}

		for _, name := range requests {
			found := false
			for _, update := range updates {
				if update.Name == name.Name {
					found = true
					break
				}
			}

			if !found {
				updates = append(updates, account_api.UpdateCheckExtensionCompatibility{
					Name: name.Name,
					Status: account_api.UpdateCheckExtensionCompatibilityStatus{
						Label: "Not available in Store",
					},
				})
			}
		}

		return compatLoadedMsg{updates: updates}
	}
}

func (m wizardModel) startTask() (tea.Model, tea.Cmd) {
	if m.currentTask >= len(m.tasks) {
		m.finished = true
		m.phase = phaseDone
		return m, nil
	}

	m.tasks[m.currentTask].status = taskRunning

	switch m.currentTask {
	case taskBackup:
		return m, m.runBackup()
	case taskCleanup:
		return m, m.runCleanup()
	case taskPlugins:
		return m, m.runRemovePlugins()
	case taskComposerJSON:
		return m, m.runUpdateComposer()
	case taskComposerUpdate:
		return m.startComposerUpdate()
	case taskSystemPrepare:
		return m.startSystemUpdate("system:update:prepare", taskSystemPrepare)
	case taskSystemFinish:
		return m.startSystemUpdate("system:update:finish", taskSystemFinish)
	}

	return m, nil
}

func (m wizardModel) runBackup() tea.Cmd {
	composerJSONPath := m.opts.ComposerJSONPath
	idx := taskBackup
	return func() tea.Msg {
		data, err := os.ReadFile(composerJSONPath)
		if err != nil {
			return taskCompleteMsg{task: idx, err: fmt.Errorf("read composer.json: %w", err)}
		}
		return taskCompleteMsg{task: idx, detail: fmt.Sprintf("%d bytes", len(data)), composerBackup: data}
	}
}

func (m wizardModel) runCleanup() tea.Cmd {
	projectRoot := m.opts.ProjectRoot
	idx := taskCleanup
	return func() tea.Msg {
		if err := flexmigrator.CleanupByHash(projectRoot); err != nil {
			return taskCompleteMsg{task: idx, err: err}
		}
		return taskCompleteMsg{task: idx}
	}
}

func (m wizardModel) runRemovePlugins() tea.Cmd {
	composerJSONPath := m.opts.ComposerJSONPath
	target := m.targetVersion
	idx := taskPlugins
	restore := m.composerBackup
	registry := m.opts.Registry
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := ResolveIncompatiblePlugins(ctx, composerJSONPath, target, registry)
		if err != nil {
			_ = os.WriteFile(composerJSONPath, restore, 0o644)
			return taskCompleteMsg{task: idx, err: err}
		}
		if result == nil {
			result = &ResolveResult{}
		}
		bumped := len(result.Bumped())
		removed := len(result.Removed())
		detail := "no incompatibilities"
		switch {
		case bumped > 0 && removed > 0:
			detail = fmt.Sprintf("bumped %d, removed %d", bumped, removed)
		case bumped > 0:
			detail = fmt.Sprintf("bumped %d to a compatible version", bumped)
		case removed > 0:
			detail = fmt.Sprintf("removed %d (no compatible release)", removed)
		}
		return taskCompleteMsg{task: idx, detail: detail, pluginActions: result}
	}
}

func (m wizardModel) runUpdateComposer() tea.Cmd {
	composerJSONPath := m.opts.ComposerJSONPath
	target := m.targetVersion
	idx := taskComposerJSON
	restore := m.composerBackup
	return func() tea.Msg {
		if err := UpdateComposerJson(composerJSONPath, target); err != nil {
			_ = os.WriteFile(composerJSONPath, restore, 0o644)
			return taskCompleteMsg{task: idx, err: err}
		}
		return taskCompleteMsg{task: idx, detail: "pinned to " + target}
	}
}

func (m wizardModel) startComposerUpdate() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelExecution = cancel

	ch := make(chan string, streamBufferSize)
	m.logChan = ch
	m.logLines = nil

	args := []string{
		"update",
		"--no-interaction",
		"--no-scripts",
		"--with-all-dependencies",
		"-v",
	}
	p := m.opts.Executor.ComposerCommand(ctx, args...)

	restore := m.composerBackup
	composerJSONPath := m.opts.ComposerJSONPath
	idx := taskComposerUpdate

	doneCmd := func() tea.Msg {
		err := streamCmdOutput(p.Cmd, ch, true)
		if err != nil {
			_ = os.WriteFile(composerJSONPath, restore, 0o644)
		}
		return taskCompleteMsg{task: idx, err: err}
	}

	return m, tea.Batch(m.readNextLog(), doneCmd)
}

func (m wizardModel) startSystemUpdate(consoleCmd string, idx int) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelExecution = cancel

	ch := make(chan string, streamBufferSize)
	m.logChan = ch
	m.logLines = nil

	p := m.opts.Executor.ConsoleCommand(ctx, consoleCmd, "--no-interaction")

	doneCmd := func() tea.Msg {
		err := streamCmdOutput(p.Cmd, ch, true)
		return taskCompleteMsg{task: idx, err: err}
	}

	return m, tea.Batch(m.readNextLog(), doneCmd)
}

// streamCmdOutput starts cmd, fans stdout (or stderr) lines into ch, and
// closes ch when done. The returned error is the process exit error, if any.
func streamCmdOutput(cmd *exec.Cmd, ch chan<- string, useStdout bool) error {
	var pipe io.Reader
	var err error
	if useStdout {
		pipe, err = cmd.StdoutPipe()
		if err == nil {
			cmd.Stderr = cmd.Stdout
		}
	} else {
		pipe, err = cmd.StderrPipe()
		if err == nil {
			cmd.Stdout = cmd.Stderr
		}
	}
	if err != nil {
		close(ch)
		return err
	}

	if err := cmd.Start(); err != nil {
		close(ch)
		return err
	}

	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		ch <- scanner.Text()
	}
	close(ch)

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return err
	}
	return cmd.Wait()
}

// --- View ---

func (m wizardModel) View() tea.View {
	v := tea.NewView(m.viewContent())
	v.AltScreen = true
	return v
}

func (m wizardModel) viewContent() string {
	switch m.phase {
	case phaseWelcome:
		return m.viewWelcome()
	case phaseSelectVersion:
		return m.viewSelectVersion()
	case phaseCompatCheck:
		return m.viewCompatCheck()
	case phaseCompatResult:
		return m.viewCompatResult()
	case phaseReview:
		return m.viewReview()
	case phaseRunning:
		return m.viewRunning()
	case phaseDone:
		return m.viewDone()
	}
	return ""
}

func (m wizardModel) totalSteps() int {
	if len(m.opts.Extensions) == 0 {
		return 3 // Select version, Review, Run
	}
	return 4 // + Compatibility check
}

func (m wizardModel) stepNum(p phase) int {
	switch p {
	case phaseSelectVersion:
		return 1
	case phaseCompatCheck, phaseCompatResult:
		return 2
	case phaseReview:
		if len(m.opts.Extensions) == 0 {
			return 2
		}
		return 3
	case phaseRunning:
		if len(m.opts.Extensions) == 0 {
			return 3
		}
		return 4
	case phaseWelcome, phaseDone:
		return 0
	}
	return 0
}

func (m wizardModel) viewWelcome() string {
	var b strings.Builder
	b.WriteString(tui.TextBadge("Upgrade"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.BrandColor).Render("Upgrade Shopware to a newer version"))
	b.WriteString("\n\n")
	b.WriteString(tui.DimStyle.Render("This wizard mirrors the shopware/web-installer flow:"))
	b.WriteString("\n\n")
	for _, line := range []string{
		"Back up composer.json before any change",
		"Clean up stale recipe-managed files (md5-matched)",
		"Drop incompatible custom plugins from composer.json",
		"Rewrite composer.json to pin the target version",
		"Run composer update --with-all-dependencies --no-scripts",
		"Run bin/console system:update:prepare + finish",
	} {
		b.WriteString(tui.DimStyle.Render("  • "))
		b.WriteString(tui.LabelStyle.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(tui.SectionDivider(tui.PhaseCardWidth - 6))
	b.WriteString(tui.KVRow("Current version", tui.BoldText.Render(m.opts.CurrentVersion.String())))
	b.WriteString(tui.KVRow("Project root", tui.DimStyle.Render(m.opts.ProjectRoot)))
	if len(m.opts.Extensions) > 0 {
		b.WriteString(tui.KVRow("Installed extensions", tui.LabelStyle.Render(fmt.Sprintf("%d", len(m.opts.Extensions)))))
	}
	b.WriteString("\n")
	b.WriteString(renderConfirmButtons("Begin upgrade", "Cancel", m.confirmYes))
	b.WriteString("\n\n")
	b.WriteString(m.footer(
		tui.Shortcut{Key: "←/→", Label: "Select"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	))
	return tui.RenderPhaseCardCowsay("Let's get this Shopware up to date!", b.String())
}

func (m wizardModel) viewSelectVersion() string {
	var b strings.Builder
	b.WriteString(stepBadge(m.stepNum(phaseSelectVersion), m.totalSteps()))
	b.WriteString("\n\n")

	opts := make([]tui.SelectOption, len(m.opts.UpdateVersions))
	for i, v := range m.opts.UpdateVersions {
		detail := ""
		if i == 0 {
			detail = "latest"
		}
		opts[i] = tui.SelectOption{Label: v, Detail: detail}
	}
	b.WriteString(tui.RenderSelectList(
		"Select target version",
		"Pick the Shopware version to upgrade to. Next-major releases are listed first.",
		opts,
		m.versionCursor,
	))
	b.WriteString("\n\n")
	b.WriteString(m.footer(
		tui.Shortcut{Key: "↑/↓", Label: "Select"},
		tui.Shortcut{Key: "enter", Label: "Continue"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) viewCompatCheck() string {
	var b strings.Builder
	b.WriteString(stepBadge(m.stepNum(phaseCompatCheck), m.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Checking extension compatibility"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render(fmt.Sprintf("Asking the Shopware store about %d installed extension(s) against %s…", len(m.opts.Extensions), m.targetVersion)))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View() + " " + tui.DimStyle.Render("fetching compatibility"))
	b.WriteString("\n\n")
	b.WriteString(m.footer(tui.Shortcut{Key: "ctrl+c", Label: "Cancel"}))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) viewCompatResult() string {
	var b strings.Builder
	b.WriteString(stepBadge(m.stepNum(phaseCompatResult), m.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Extension compatibility"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render(fmt.Sprintf("Upgrade to %s", m.targetVersion)))
	b.WriteString("\n\n")

	switch {
	case m.compatErr != nil:
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ErrorColor).Render("Compatibility lookup failed: " + m.compatErr.Error()))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("You may still proceed; the wizard cannot guarantee extensions will install."))
		b.WriteString("\n\n")
	case len(m.compatUpdates) == 0:
		b.WriteString(tui.DimStyle.Render("No store-managed extensions to check."))
		b.WriteString("\n\n")
	default:
		for _, u := range m.compatUpdates {
			icon := tui.Checkmark
			if u.Status.IsBlocker() {
				icon = lipgloss.NewStyle().Foreground(tui.ErrorColor).Bold(true).Render("✗")
			}
			b.WriteString("  ")
			b.WriteString(icon)
			b.WriteString("  ")
			b.WriteString(tui.LabelStyle.Render(u.Name))
			b.WriteString(tui.DimStyle.Render(" — " + u.Status.Label))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	if m.compatHasBlock {
		b.WriteString(lipgloss.NewStyle().Foreground(tui.WarnColor).Bold(true).Render("⚠ Some extensions are not compatible yet."))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("Continuing may break those extensions until they release updates."))
		b.WriteString("\n\n")
	}

	b.WriteString(renderConfirmButtons("Continue", "Cancel", m.confirmYes))
	b.WriteString("\n\n")
	b.WriteString(m.footer(
		tui.Shortcut{Key: "←/→", Label: "Select"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) viewReview() string {
	var b strings.Builder
	b.WriteString(stepBadge(m.stepNum(phaseReview), m.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Review"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("Confirm to apply the following changes."))
	b.WriteString("\n\n")
	b.WriteString(tui.KVRow("From", tui.BoldText.Render(m.opts.CurrentVersion.String())))
	b.WriteString(tui.KVRow("To", lipgloss.NewStyle().Foreground(tui.SuccessColor).Bold(true).Render(m.targetVersion)))
	if m.opts.Executor != nil {
		b.WriteString(tui.KVRow("Executor", tui.LabelStyle.Render(m.opts.Executor.Type())))
	}
	b.WriteString(tui.SectionDivider(tui.PhaseCardWidth - 6))
	b.WriteString(tui.DimStyle.Render("Tasks to be executed:"))
	b.WriteString("\n")
	for _, t := range m.tasks {
		b.WriteString(tui.DimStyle.Render("  • "))
		b.WriteString(tui.LabelStyle.Render(t.label))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(tui.WarnColor).Render("⚠  Commit your changes before continuing."))
	b.WriteString("\n\n")
	b.WriteString(renderConfirmButtons("Start upgrade", "Cancel", m.confirmYes))
	b.WriteString("\n\n")
	b.WriteString(m.footer(
		tui.Shortcut{Key: "←/→", Label: "Select"},
		tui.Shortcut{Key: "enter", Label: "Confirm"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) viewRunning() string {
	var b strings.Builder
	b.WriteString(stepBadge(m.stepNum(phaseRunning), m.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render(fmt.Sprintf("Upgrading to %s", m.targetVersion)))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render("This may take a few minutes. Live output shown below."))
	b.WriteString("\n\n")

	for i, t := range m.tasks {
		b.WriteString(m.renderTaskLine(i, t))
		b.WriteString("\n")
	}

	if len(m.logLines) > 0 {
		b.WriteString("\n")
		b.WriteString(tui.SectionDivider(tui.PhaseCardWidth - 6))
		b.WriteString(tui.DimStyle.Render("Output:"))
		b.WriteString("\n")
		for _, line := range m.logLines {
			b.WriteString(tui.DimStyle.Render("  " + truncate(line, tui.PhaseCardWidth-10)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.footer(tui.Shortcut{Key: "ctrl+c", Label: "Cancel"}))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) viewDone() string {
	var b strings.Builder

	if m.finalErr != nil {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.ErrorColor).Render("✗ Upgrade failed"))
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ErrorColor).Render(m.finalErr.Error()))
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("composer.json was restored from the backup taken before the upgrade."))
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.SuccessColor).Render(fmt.Sprintf("✓ Upgraded to Shopware %s", m.targetVersion)))
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("All tasks completed. Verify your shop and run your test suite."))

		if m.pluginActions != nil {
			bumped := m.pluginActions.Bumped()
			removed := m.pluginActions.Removed()

			if len(bumped) > 0 {
				b.WriteString("\n\n")
				b.WriteString(tui.BoldText.Render("Bumped plugin constraints:"))
				b.WriteString("\n")
				for _, action := range bumped {
					b.WriteString(tui.DimStyle.Render("  • "))
					b.WriteString(tui.LabelStyle.Render(action.Name))
					b.WriteString("  ")
					b.WriteString(tui.DimStyle.Render(action.OldConstraint))
					b.WriteString(" → ")
					b.WriteString(lipgloss.NewStyle().Foreground(tui.SuccessColor).Render(action.NewConstraint))
					b.WriteString("\n")
				}
			}

			if len(removed) > 0 {
				b.WriteString("\n\n")
				b.WriteString(tui.BoldText.Render("Removed incompatible custom plugins:"))
				b.WriteString("\n")
				for _, action := range removed {
					b.WriteString(tui.DimStyle.Render("  • "))
					b.WriteString(tui.LabelStyle.Render(action.Name))
					if action.Reason != "" {
						b.WriteString(tui.DimStyle.Render(" (" + action.Reason + ")"))
					}
					b.WriteString("\n")
				}
				b.WriteString(tui.DimStyle.Render("Re-require them in composer.json once they publish compatible versions."))
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(tui.SectionDivider(tui.PhaseCardWidth - 6))
	for i, t := range m.tasks {
		b.WriteString(m.renderTaskLine(i, t))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(m.footer(tui.Shortcut{Key: "enter", Label: "Close"}))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) renderTaskLine(i int, t task) string {
	var icon string
	switch t.status {
	case taskRunning:
		icon = m.spinner.View()
	case taskDone:
		icon = tui.Checkmark
	case taskFailed:
		icon = lipgloss.NewStyle().Foreground(tui.ErrorColor).Bold(true).Render("✗")
	case taskSkipped:
		icon = tui.DimStyle.Render("·")
	case taskPending:
		icon = tui.DimStyle.Render("○")
	default:
		icon = tui.DimStyle.Render("○")
	}

	style := tui.LabelStyle
	if t.status == taskPending {
		style = tui.DimStyle
	}

	line := fmt.Sprintf("  %s  %s", icon, style.Render(t.label))
	if t.detail != "" {
		line += " " + tui.DimStyle.Render("("+t.detail+")")
	}
	if i == m.currentTask && t.status == taskRunning {
		line = lipgloss.NewStyle().Bold(true).Render(line)
	}
	return line
}

func (m wizardModel) footer(shortcuts ...tui.Shortcut) string {
	return tui.ShortcutBar(shortcuts...)
}

func stepBadge(stepNum, totalSteps int) string {
	if stepNum == 0 {
		return tui.TextBadge("Upgrade")
	}
	return tui.TextBadge(fmt.Sprintf("Step %d/%d", stepNum, totalSteps))
}

func renderConfirmButtons(yesLabel, noLabel string, yesActive bool) string {
	yesStyle := lipgloss.NewStyle().Foreground(tui.TextColor).Background(tui.BrandColor).Padding(0, 2)
	noStyle := lipgloss.NewStyle().Foreground(tui.MutedColor).Background(tui.SubtleBgColor).Padding(0, 2)

	var yes, no string
	if yesActive {
		yes = yesStyle.Render(yesLabel)
		no = noStyle.Render(noLabel)
	} else {
		yes = noStyle.Render(yesLabel)
		no = yesStyle.Render(noLabel)
	}
	return yes + "  " + no
}

func truncate(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return s
	}
	if len([]rune(s)) <= maxRunes {
		return s
	}
	r := []rune(s)
	return string(r[:maxRunes-1]) + "…"
}

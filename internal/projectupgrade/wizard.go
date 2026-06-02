package projectupgrade

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/shyim/go-version"

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

// maxVisibleVersions caps how many upgrade targets the version select list
// shows at once so the card keeps a fixed height when many versions exist.
const maxVisibleVersions = 10

// WizardOptions configures a single run of the upgrade wizard.
type WizardOptions struct {
	ProjectRoot      string
	ComposerJSONPath string
	CurrentVersion   *version.Version
	UpdateVersions   []string
	Executor         executor.Executor
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

// Stable indices into model.tasks. The flow is: rewrite composer.json,
// have composer pull the new vendor code, then let shopware-deployment-helper
// drive the install/update lifecycle (system:update:prepare, migrations,
// system:update:finish, theme compile, etc.) in one pass.
const (
	taskCleanup = iota
	taskPlugins
	taskComposerJSON
	taskComposerUpdate
	taskDeploymentHelper
)

// wizardMsg variants advance the upgrade state machine.
type (
	compatLoadedMsg struct {
		report CompatReport
		err    error
	}
	taskCompleteMsg struct {
		task          int
		err           error
		detail        string
		pluginActions *ResolveResult
		// output is the full captured subprocess output, set for streaming
		// tasks so the complete log survives independent of log-line event
		// ordering. Used to render the error tail on failure.
		output []string
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

	versionList     *tui.SelectList
	targetVersion   string
	confirmYes      bool
	pluginActions   *ResolveResult
	compatReport    CompatReport
	compatHasBlock  bool
	compatErr       error
	tasks           []task
	currentTask     int
	logLines        []string
	fullLog         []string
	logChan         chan string
	finalErr        error
	finished        bool
	spinner         spinner.Model
	compatLoading   bool
	cancelExecution context.CancelFunc
	width           int
	height          int
}

// WizardResult is the outcome of a single RunWizard invocation.
type WizardResult struct {
	// TargetVersion is the version the user selected (empty if cancelled
	// before selecting).
	TargetVersion string
	// Success is true when every upgrade task completed.
	Success bool
	// FailureLog holds the full captured output of the task that failed. It
	// is nil on success or when the failure produced no subprocess output, so
	// callers can print it verbatim to give the user the complete log the
	// alt-screen could not retain.
	FailureLog []string
}

// RunWizard runs the interactive upgrade wizard. It returns the result and any
// error encountered. A user cancellation returns ErrCancelled.
func RunWizard(opts WizardOptions) (WizardResult, error) {
	s := spinner.New(
		spinner.WithSpinner(spinner.Dot),
		spinner.WithStyle(lipgloss.NewStyle().Foreground(tui.BrandColor)),
	)

	versionOptions := make([]tui.SelectOption, len(opts.UpdateVersions))
	for i, v := range opts.UpdateVersions {
		detail := ""
		if i == 0 {
			detail = "latest"
		}
		versionOptions[i] = tui.SelectOption{Label: v, Detail: detail}
	}

	m := wizardModel{
		opts:       opts,
		phase:      phaseWelcome,
		confirmYes: true,
		versionList: tui.NewSelectList(
			"Select target version",
			"Pick the Shopware version to upgrade to. Next-major releases are listed first.",
			versionOptions,
			maxVisibleVersions,
		),
		spinner: s,
		tasks:   defaultTasks(),
	}

	prog := tea.NewProgram(m)
	final, err := prog.Run()
	if err != nil {
		return WizardResult{}, err
	}

	fm, _ := final.(wizardModel)
	if fm.cancelExecution != nil {
		fm.cancelExecution()
	}

	if !fm.finished {
		return WizardResult{TargetVersion: fm.targetVersion}, ErrCancelled
	}

	result := WizardResult{
		TargetVersion: fm.targetVersion,
		Success:       fm.finalErr == nil,
	}
	if fm.finalErr != nil {
		result.FailureLog = fm.fullLog
	}
	return result, fm.finalErr
}

// ErrCancelled is returned when the user exits the wizard before the upgrade
// completes (e.g. via q / ctrl+c or selecting the cancel button).
var ErrCancelled = errors.New("upgrade cancelled by user")

func defaultTasks() []task {
	return []task{
		{label: "Clean up stale recipe files"},
		{label: "Resolve incompatible custom plugins"},
		{label: "Rewrite composer.json"},
		{label: "composer update --with-all-dependencies"},
		{label: "vendor/bin/shopware-deployment-helper run"},
	}
}

func (m wizardModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m wizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		return m.updateKey(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case compatLoadedMsg:
		m.compatLoading = false
		m.compatErr = msg.err
		m.compatReport = msg.report
		m.compatHasBlock = !msg.report.OK
		m.phase = phaseCompatResult
		m.confirmYes = !m.compatHasBlock
		return m, nil

	case startNextTaskMsg:
		return m.startTask()

	case taskCompleteMsg:
		if msg.pluginActions != nil {
			m.pluginActions = msg.pluginActions
		}
		if msg.output != nil {
			m.fullLog = msg.output
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
	if m.versionList.HandleKey(key) {
		return m, nil
	}

	switch key {
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		selected, ok := m.versionList.Selected()
		if !ok {
			return m, nil
		}
		m.targetVersion = selected.Label
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
	// fullLog keeps the complete subprocess output so it can be surfaced on
	// failure (both in the done view and printed to the terminal after the
	// alt-screen tears down). logLines is the capped window shown live.
	m.fullLog = append(m.fullLog, line)
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

// loadCompatibility asks composer (dry run) whether the project can be upgraded
// to the target version, so the user sees composer's own verdict before
// applying anything.
func (m wizardModel) loadCompatibility() tea.Cmd {
	composerJsonPath := m.opts.ComposerJSONPath
	targetVersion := m.targetVersion
	exec := m.opts.Executor

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		report, err := DryRunRequire(ctx, exec, composerJsonPath, targetVersion)
		if err != nil {
			return compatLoadedMsg{err: err}
		}
		return compatLoadedMsg{report: report}
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
	case taskCleanup:
		return m, m.runCleanup()
	case taskPlugins:
		return m, m.runRemovePlugins()
	case taskComposerJSON:
		return m, m.runUpdateComposer()
	case taskComposerUpdate:
		return m.startComposerUpdate()
	case taskDeploymentHelper:
		return m.startDeploymentHelper()
	}

	return m, nil
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
	exec := m.opts.Executor
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		result, err := ApplyRequire(ctx, exec, composerJSONPath, target)
		if err != nil {
			return taskCompleteMsg{task: idx, err: err}
		}
		if result == nil {
			result = &ResolveResult{}
		}
		removed := len(result.Removed())
		detail := "composer resolved all plugins"
		if removed > 0 {
			detail = fmt.Sprintf("removed %d (composer could not resolve)", removed)
		}
		return taskCompleteMsg{task: idx, detail: detail, pluginActions: result}
	}
}

func (m wizardModel) runUpdateComposer() tea.Cmd {
	composerJSONPath := m.opts.ComposerJSONPath
	target := m.targetVersion
	idx := taskComposerJSON
	return func() tea.Msg {
		if err := UpdateComposerJson(composerJSONPath, target); err != nil {
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
	m.fullLog = nil

	args := []string{
		"update",
		"--no-interaction",
		"--no-scripts",
		"--with-all-dependencies",
		"-v",
	}
	p := m.opts.Executor.ComposerCommand(ctx, args...)

	idx := taskComposerUpdate

	doneCmd := func() tea.Msg {
		output, err := streamCmdOutput(p.Cmd, ch, true)
		return taskCompleteMsg{task: idx, err: err, output: output}
	}

	return m, tea.Batch(m.readNextLog(), doneCmd)
}

func (m wizardModel) startDeploymentHelper() (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelExecution = cancel

	ch := make(chan string, streamBufferSize)
	m.logChan = ch
	m.logLines = nil
	m.fullLog = nil

	p := m.opts.Executor.PHPCommand(ctx, "vendor/bin/shopware-deployment-helper", "run")

	idx := taskDeploymentHelper

	doneCmd := func() tea.Msg {
		output, err := streamCmdOutput(p.Cmd, ch, true)
		return taskCompleteMsg{task: idx, err: err, output: output}
	}

	return m, tea.Batch(m.readNextLog(), doneCmd)
}

// streamCmdOutput starts cmd, fans stdout (or stderr) lines into ch, and
// closes ch when done. It also returns the complete captured output so the
// caller can surface it on failure regardless of how many lines the live
// view kept. The returned error is the process exit error, if any.
func streamCmdOutput(cmd *exec.Cmd, ch chan<- string, useStdout bool) ([]string, error) {
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
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		close(ch)
		return nil, err
	}

	var captured []string
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		captured = append(captured, line)
		ch <- line
	}
	close(ch)

	if err := scanner.Err(); err != nil {
		_ = cmd.Wait()
		return captured, err
	}
	return captured, cmd.Wait()
}

// --- View ---

func (m wizardModel) View() tea.View {
	content := m.viewContent()
	if m.width > 0 && m.height > 0 {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	v := tea.NewView(content)
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
	return 4 // Select version, Compatibility check, Review, Run
}

func (m wizardModel) stepNum(p phase) int {
	switch p {
	case phaseSelectVersion:
		return 1
	case phaseCompatCheck, phaseCompatResult:
		return 2
	case phaseReview:
		return 3
	case phaseRunning:
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
		"Clean up stale recipe-managed files (md5-matched)",
		"Let composer require the target version (dropping plugins it can't resolve)",
		"Rewrite composer.json to pin the target version and ensure shopware/deployment-helper",
		"Run composer update --with-all-dependencies --no-scripts",
		"Run vendor/bin/shopware-deployment-helper run",
	} {
		b.WriteString(tui.DimStyle.Render("  • "))
		b.WriteString(tui.LabelStyle.Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(tui.SectionDivider(tui.PhaseCardWidth - 6))
	b.WriteString(tui.KVRow("Current version", tui.BoldText.Render(m.opts.CurrentVersion.String())))
	b.WriteString(tui.KVRow("Project root", tui.DimStyle.Render(m.opts.ProjectRoot)))
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

	b.WriteString(m.versionList.View())
	b.WriteString("\n\n")

	shortcuts := append(m.versionList.Shortcuts(),
		tui.Shortcut{Key: "enter", Label: "Continue"},
		tui.Shortcut{Key: "ctrl+c", Label: "Exit"},
	)
	b.WriteString(m.footer(shortcuts...))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) viewCompatCheck() string {
	var b strings.Builder
	b.WriteString(stepBadge(m.stepNum(phaseCompatCheck), m.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Checking plugin compatibility"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render(fmt.Sprintf("Asking composer to resolve %s…", m.targetVersion)))
	b.WriteString("\n\n")
	b.WriteString(m.spinner.View() + " " + tui.DimStyle.Render("composer require --dry-run"))
	b.WriteString("\n\n")
	b.WriteString(m.footer(tui.Shortcut{Key: "ctrl+c", Label: "Cancel"}))
	return tui.RenderPhaseCard(b.String())
}

func (m wizardModel) viewCompatResult() string {
	var b strings.Builder
	b.WriteString(stepBadge(m.stepNum(phaseCompatResult), m.totalSteps()))
	b.WriteString("\n\n")
	b.WriteString(tui.TitleStyle.Render("Plugin compatibility"))
	b.WriteString("\n")
	b.WriteString(tui.DimStyle.Render(fmt.Sprintf("Upgrade to %s", m.targetVersion)))
	b.WriteString("\n\n")

	switch {
	case m.compatErr != nil:
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ErrorColor).Render("Compatibility check failed: " + m.compatErr.Error()))
		b.WriteString("\n")
		b.WriteString(tui.DimStyle.Render("You may still proceed; the wizard cannot guarantee plugins will install."))
		b.WriteString("\n\n")
	case m.compatReport.OK:
		b.WriteString("  ")
		b.WriteString(tui.Checkmark)
		b.WriteString("  ")
		b.WriteString(tui.LabelStyle.Render("composer can resolve this upgrade"))
		b.WriteString("\n\n")
	default:
		b.WriteString(lipgloss.NewStyle().Foreground(tui.ErrorColor).Bold(true).Render("✗ composer could not resolve this upgrade."))
		b.WriteString("\n\n")
		if len(m.compatReport.BlockingPlugins) > 0 {
			b.WriteString(tui.DimStyle.Render("These plugins block the upgrade and will be removed from composer.json so it can proceed:"))
			b.WriteString("\n")
			for _, name := range m.compatReport.BlockingPlugins {
				b.WriteString("  ")
				b.WriteString(lipgloss.NewStyle().Foreground(tui.ErrorColor).Render("✗"))
				b.WriteString("  ")
				b.WriteString(tui.LabelStyle.Render(name))
				b.WriteString("\n")
			}
			b.WriteString(tui.DimStyle.Render("Re-require them once they publish a compatible release."))
			b.WriteString("\n\n")
		}
		for _, line := range lastLines(m.compatReport.Output, 12) {
			b.WriteString(tui.DimStyle.Render("  " + line))
			b.WriteString("\n")
		}
		b.WriteString("\n")
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

		if tail := lastLines(m.fullLog, maxLogLines); len(tail) > 0 {
			b.WriteString(tui.BoldText.Render("Last output:"))
			b.WriteString("\n")
			for _, line := range tail {
				b.WriteString(tui.DimStyle.Render("  " + truncate(line, tui.PhaseCardWidth-10)))
				b.WriteString("\n")
			}
			b.WriteString("\n")
			b.WriteString(tui.DimStyle.Render("The full log is printed below the wizard after you close it."))
			b.WriteString("\n\n")
		}

		b.WriteString(tui.DimStyle.Render("composer.json and vendor are left as-is; run `git checkout composer.json composer.lock` to revert."))
	} else {
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(tui.SuccessColor).Render(fmt.Sprintf("✓ Upgraded to Shopware %s", m.targetVersion)))
		b.WriteString("\n\n")
		b.WriteString(tui.DimStyle.Render("All tasks completed. Verify your shop and run your test suite."))

		if m.pluginActions != nil {
			removed := m.pluginActions.Removed()

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

// lastLines returns up to n trailing lines of lines.
func lastLines(lines []string, n int) []string {
	if n <= 0 || len(lines) <= n {
		return lines
	}
	return lines[len(lines)-n:]
}

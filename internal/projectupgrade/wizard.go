package projectupgrade

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/flexmigrator"
	"github.com/shopware/shopware-cli/internal/packagist"
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

// maxVisibleExtensions caps how many extension queue rows are shown at once;
// the window scrolls with the cursor.
const maxVisibleExtensions = 8

// WizardOptions configures a single run of the upgrade wizard.
type WizardOptions struct {
	ProjectRoot      string
	ComposerJSONPath string
	CurrentVersion   *version.Version
	UpdateVersions   []string
	Executor         executor.Executor
	// AllowDirty downgrades the "git working tree clean" preflight check to
	// skipped (set via --allow-dirty).
	AllowDirty bool
	// AllowNonComposer downgrades the "plugins managed by composer" preflight
	// check to skipped (set via --allow-non-composer).
	AllowNonComposer bool
}

type phase int

const (
	phaseWelcome phase = iota
	phasePreflight
	phaseSelectVersion
	phasePrepare
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
	preflightDoneMsg struct {
		results []PreflightResult
	}
	compatLoadedMsg struct {
		report         CompatReport
		queue          []ExtensionRow
		phpRequirement string
		err            error
	}
	reportWrittenMsg struct {
		path string
		err  error
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

	preflight        []PreflightResult
	preflightLoading bool

	versionList   *tui.SelectList
	targetVersion string
	confirmYes    bool

	compatLoading bool
	compatErr     error
	compatReport  CompatReport

	extQueue  []ExtensionRow
	extCursor int
	// overlayOpen shows the detail overlay for the extension under the cursor.
	overlayOpen bool
	// markedRemove records the user's decision to drop a blocked extension
	// from composer.json during the upgrade. Survives rechecks.
	markedRemove map[string]bool

	phpRequirement string
	reportPath     string
	reportErr      error

	pluginActions   *ResolveResult
	tasks           []task
	currentTask     int
	logLines        []string
	fullLog         []string
	logChan         chan string
	finalErr        error
	finished        bool
	spinner         spinner.Model
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
	// ReportPath is the upgrade report file the user exported, if any.
	ReportPath string
	// FailureLog holds the full captured output of the task that failed. It
	// is nil on success or when the failure produced no subprocess output, so
	// callers can print it verbatim to give the user the complete log the
	// alt-screen could not retain.
	FailureLog []string
}

// RunWizard runs the interactive upgrade wizard. It returns the result and any
// error encountered. A user cancellation returns ErrCancelled.
func RunWizard(opts WizardOptions) (WizardResult, error) {
	m := newWizardModel(opts)

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
		return WizardResult{TargetVersion: fm.targetVersion, ReportPath: fm.reportPath}, ErrCancelled
	}

	result := WizardResult{
		TargetVersion: fm.targetVersion,
		Success:       fm.finalErr == nil,
		ReportPath:    fm.reportPath,
	}
	if fm.finalErr != nil {
		result.FailureLog = fm.fullLog
	}
	return result, fm.finalErr
}

func newWizardModel(opts WizardOptions) wizardModel {
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

	return wizardModel{
		opts:       opts,
		phase:      phaseWelcome,
		confirmYes: true,
		versionList: tui.NewSelectList(
			"Select target version",
			"Pick the Shopware version to upgrade to. Next-major releases are listed first.",
			versionOptions,
			maxVisibleVersions,
		),
		spinner:      s,
		tasks:        defaultTasks(),
		markedRemove: map[string]bool{},
	}
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

	case preflightDoneMsg:
		m.preflightLoading = false
		m.preflight = msg.results
		return m, nil

	case compatLoadedMsg:
		m.compatLoading = false
		m.compatErr = msg.err
		m.compatReport = msg.report
		m.extQueue = msg.queue
		m.extCursor = 0
		m.overlayOpen = false
		if msg.phpRequirement != "" {
			m.phpRequirement = msg.phpRequirement
		}
		m.applyRemovalMarks()
		return m, nil

	case reportWrittenMsg:
		m.reportPath = msg.path
		m.reportErr = msg.err
		if msg.err != nil {
			m.reportPath = ""
		}
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

// applyRemovalMarks re-applies the user's remove decisions to a freshly built
// queue. A mark only sticks while the extension still blocks the upgrade; if
// a recheck finds a compatible release, the decision is obsolete and dropped.
func (m *wizardModel) applyRemovalMarks() {
	for i := range m.extQueue {
		row := &m.extQueue[i]
		if m.markedRemove[row.Name] {
			if row.State == ExtensionBlocked {
				markRowRemoved(row)
			} else {
				delete(m.markedRemove, row.Name)
			}
		}
	}
}

func markRowRemoved(row *ExtensionRow) {
	row.State = ExtensionRemove
	row.Result = "will be removed during the upgrade"
}

func unmarkRowRemoved(row *ExtensionRow) {
	row.State = ExtensionBlocked
	row.Result = "no compatible release"
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
	case phasePreflight:
		return m.updatePreflight(key)
	case phaseSelectVersion:
		return m.updateSelectVersion(key)
	case phasePrepare:
		return m.updatePrepare(key)
	case phaseReview:
		return m.updateReview(key)
	case phaseRunning:
		return m, nil
	case phaseDone:
		switch key {
		case "e":
			return m, m.writeReport()
		case "q", "enter", "esc":
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
		return m.startPreflight()
	}
	return m, nil
}

func (m wizardModel) startPreflight() (tea.Model, tea.Cmd) {
	m.phase = phasePreflight
	m.preflightLoading = true
	m.preflight = nil
	return m, tea.Batch(m.spinner.Tick, m.runPreflight())
}

func (m wizardModel) runPreflight() tea.Cmd {
	opts := m.opts
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		return preflightDoneMsg{results: RunPreflightChecks(ctx, opts)}
	}
}

func (m wizardModel) updatePreflight(key string) (tea.Model, tea.Cmd) {
	if m.preflightLoading {
		return m, nil
	}

	switch key {
	case "r":
		return m.startPreflight()
	case "q", "esc":
		return m, tea.Quit
	case "enter":
		if PreflightBlocked(m.preflight) {
			return m, nil
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
		return m.startCompatCheck()
	}
	return m, nil
}

func (m wizardModel) startCompatCheck() (tea.Model, tea.Cmd) {
	m.phase = phasePrepare
	m.compatLoading = true
	m.compatErr = nil
	m.overlayOpen = false
	return m, tea.Batch(m.spinner.Tick, m.loadCompatibility())
}

func (m wizardModel) updatePrepare(key string) (tea.Model, tea.Cmd) {
	if m.compatLoading {
		return m, nil
	}

	if m.overlayOpen {
		return m.updatePrepareOverlay(key)
	}

	switch key {
	case "up", "k":
		if m.extCursor > 0 {
			m.extCursor--
		}
	case "down", "j":
		if m.extCursor < len(m.extQueue)-1 {
			m.extCursor++
		}
	case "enter":
		if len(m.extQueue) > 0 {
			m.overlayOpen = true
		}
	case "r":
		return m.startCompatCheck()
	case "c":
		if m.prepareBlocked() {
			return m, nil
		}
		m.phase = phaseReview
		m.confirmYes = true
		return m, nil
	case "q", "esc":
		return m, tea.Quit
	}
	return m, nil
}

func (m wizardModel) updatePrepareOverlay(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "enter", "q":
		m.overlayOpen = false
	case "r":
		return m.startCompatCheck()
	case "d":
		if m.extCursor < len(m.extQueue) {
			row := &m.extQueue[m.extCursor]
			switch row.State {
			case ExtensionBlocked:
				markRowRemoved(row)
				m.markedRemove[row.Name] = true
			case ExtensionRemove:
				unmarkRowRemoved(row)
				delete(m.markedRemove, row.Name)
			case ExtensionDeprecated, ExtensionUpdate, ExtensionOK:
				// The remove decision only applies to blocked extensions.
			}
		}
	}
	return m, nil
}

// prepareBlocked reports whether the upgrade may not continue yet: composer
// found extensions without a compatible release and the user has not decided
// what to do with them.
func (m wizardModel) prepareBlocked() bool {
	return CountBlockers(m.extQueue) > 0
}

// plannedRemovals returns the extensions the user decided to drop during the
// upgrade.
func (m wizardModel) plannedRemovals() []ExtensionRow {
	out := make([]ExtensionRow, 0)
	for _, row := range m.extQueue {
		if row.State == ExtensionRemove {
			out = append(out, row)
		}
	}
	return out
}

func (m wizardModel) updateReview(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "left", "h":
		m.confirmYes = true
	case "right", "l":
		m.confirmYes = false
	case "tab":
		m.confirmYes = !m.confirmYes
	case "e":
		return m, m.writeReport()
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

// loadCompatibility asks composer (dry run) whether the project can be
// upgraded to the target version and derives the extension queue from its
// verdict, so the user sees composer's own verdict before applying anything.
// It also resolves the target version's PHP requirement for the report.
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

		installed, err := RequiredPluginVersions(composerJsonPath)
		if err != nil {
			return compatLoadedMsg{report: report, err: err}
		}

		// Best-effort: the report simply omits the PHP requirement when
		// packagist cannot be queried.
		phpRequirement := ""
		if releases, relErr := packagist.GetShopwarePackageVersions(ctx); relErr == nil {
			phpRequirement = phpRequirementForVersion(releases, targetVersion)
		}

		return compatLoadedMsg{
			report:         report,
			queue:          BuildExtensionQueue(installed, report),
			phpRequirement: phpRequirement,
		}
	}
}

// phpRequirementForVersion returns the `require.php` constraint shopware/core
// declares for the given version, or "" when unknown.
func phpRequirementForVersion(releases []packagist.ComposerPackageVersion, target string) string {
	normalized := strings.TrimPrefix(target, "v")
	for _, release := range releases {
		if strings.TrimPrefix(release.Version, "v") == normalized {
			return release.Require["php"]
		}
	}
	return ""
}

// reportData assembles the exportable support report from the wizard state.
func (m wizardModel) reportData(composerJSON string) ReportData {
	env := ""
	if m.opts.Executor != nil {
		env = m.opts.Executor.Type()
	}
	return ReportData{
		CurrentVersion: m.opts.CurrentVersion.String(),
		TargetVersion:  m.targetVersion,
		Environment:    env,
		PHPConstraint:  m.phpRequirement,
		Preflight:      m.preflight,
		Extensions:     m.extQueue,
		ComposerJSON:   composerJSON,
		ComposerOutput: m.compatReport.Output,
	}
}

// writeReport exports the Markdown support report next to the project's
// composer.json.
func (m wizardModel) writeReport() tea.Cmd {
	model := m
	path := filepath.Join(m.opts.ProjectRoot, fmt.Sprintf("shopware-upgrade-report-%s.md", m.targetVersion))
	composerJSONPath := m.opts.ComposerJSONPath

	return func() tea.Msg {
		raw, err := os.ReadFile(composerJSONPath)
		if err != nil {
			return reportWrittenMsg{err: err}
		}

		content := BuildMarkdownReport(model.reportData(string(raw)))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return reportWrittenMsg{err: err}
		}
		return reportWrittenMsg{path: path}
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
	removals := m.plannedRemovals()

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		result := &ResolveResult{}

		// Apply the removal decisions the user made in the extension queue
		// before composer resolves the rest.
		for _, row := range removals {
			if err := removePluginFromComposer(composerJSONPath, row.Name); err != nil {
				return taskCompleteMsg{task: idx, err: err}
			}
			result.Actions = append(result.Actions, PluginAction{
				Name:    row.Name,
				Removed: true,
				Reason:  "removed by user decision (no compatible release)",
			})
		}

		applied, err := ApplyRequire(ctx, exec, composerJSONPath, target)
		if err != nil {
			return taskCompleteMsg{task: idx, err: err}
		}
		if applied != nil {
			result.Actions = append(result.Actions, applied.Actions...)
		}

		removed := len(result.Removed())
		detail := "composer resolved all plugins"
		if removed > 0 {
			detail = fmt.Sprintf("removed %d plugin(s)", removed)
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

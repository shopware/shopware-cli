package devtui

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/tracking"
)

// Event names and tag keys live in the tracking package (tracking.EventDev*,
// tracking.Tag*) so every Track caller shares one vocabulary; the constants
// below are values specific to the dev TUI's events.

// Values of the watcher event's "result" tag: how a watcher run ended.
const (
	watcherEndPrepFailed  = "prep_failed"
	watcherEndCrashed     = "crashed"
	watcherEndUserStopped = "user_stopped"
	watcherEndSessionEnd  = "session_end"
)

// Values of the session event's "result" tag: how the session ended.
const (
	exitStopContainers = "stop_containers"
	exitKeepRunning    = "keep_running"
	exitQuit           = "quit"
)

// telemetryState accumulates anonymous usage data for one TUI session. It is
// held by pointer on Model so Bubble Tea's value copies all share it. Tests
// construct Model directly without it, so every method is nil-safe.
type telemetryState struct {
	sessionStart time.Time
	executor     string

	tabsVisited  map[string]struct{}
	watchersUsed map[string]struct{}
	actionCount  int
	exitChoice   string
	sessionSent  bool

	installStart    time.Time
	installReported bool
	dockerStart     time.Time
	restartStart    time.Time
	watcherStarts   map[string]time.Time
	taskAction      string
	taskStart       time.Time
	healthSent      bool
}

func newTelemetryState(dockerMode bool) *telemetryState {
	executorType := "local"
	if dockerMode {
		executorType = "docker"
	}
	return &telemetryState{
		sessionStart:  time.Now(),
		executor:      executorType,
		tabsVisited:   map[string]struct{}{"overview": {}},
		watchersUsed:  map[string]struct{}{},
		watcherStarts: map[string]time.Time{},
	}
}

// trackEvent sends one anonymous usage event without blocking the UI: the
// send (including DNS resolution) runs in a goroutine with a short timeout.
func trackEvent(name string, tags map[string]string) {
	go func() {
		trackEventNow(name, tags)
	}()
}

// trackEventNow sends synchronously. Quit paths must use it — a goroutine
// started right before tea.Quit would race the process exit.
func trackEventNow(name string, tags map[string]string) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	tracking.Track(ctx, name, tags)
}

func (t *telemetryState) markTab(tab activeTab) {
	if t == nil || int(tab) >= len(tabNames) {
		return
	}
	t.tabsVisited[strings.ToLower(tabNames[tab])] = struct{}{}
}

func (t *telemetryState) countAction() {
	if t == nil {
		return
	}
	t.actionCount++
}

func (t *telemetryState) setExitChoice(choice string) {
	if t == nil {
		return
	}
	t.exitChoice = choice
}

// sessionTags builds the project.dev.session event and marks it sent, so the
// event fires at most once even when several quit paths run.
func (t *telemetryState) sessionTags() (map[string]string, bool) {
	if t == nil || t.sessionSent {
		return nil, false
	}
	t.sessionSent = true

	exit := t.exitChoice
	if exit == "" {
		exit = exitQuit
	}
	tags := map[string]string{
		tracking.TagExecutor:    t.executor,
		tracking.TagDurationMS:  durationMS(time.Since(t.sessionStart)),
		tracking.TagTabsVisited: joinSet(t.tabsVisited),
		tracking.TagActions:     strconv.Itoa(t.actionCount),
		tracking.TagResult:      exit,
	}
	if len(t.watchersUsed) > 0 {
		tags[tracking.TagWatchersUsed] = joinSet(t.watchersUsed)
	}
	return tags, true
}

func (t *telemetryState) beginInstall() {
	if t == nil {
		return
	}
	t.installStart = time.Now()
}

// installOnce reports whether an install outcome should still be sent and
// latches, so quitting the failure screen doesn't add a second event on top
// of the already-reported failure.
func (t *telemetryState) installOnce() bool {
	if t == nil || t.installReported {
		return false
	}
	t.installReported = true
	return true
}

// installTags builds the project.dev.install event. The wizard's choices are
// only included once made (an event for a run cancelled on the language step
// carries no language tag). Credentials are never sent — only whether the
// defaults were changed.
func (t *telemetryState) installTags(result string, w installWizard) map[string]string {
	tags := map[string]string{tracking.TagResult: result}
	if t != nil && !t.installStart.IsZero() {
		tags[tracking.TagDurationMS] = durationMS(time.Since(t.installStart))
	}
	if w.language != "" {
		tags[tracking.TagLanguage] = w.language
	}
	if w.currency != "" {
		tags[tracking.TagCurrency] = w.currency
	}
	if w.step == installStepCredentials || result == tracking.ResultSuccess || result == tracking.ResultFailure {
		custom := w.Username() != defaultUsername || w.Password() != "shopware"
		tags[tracking.TagCustomCredentials] = strconv.FormatBool(custom)
	}
	return tags
}

func installStepTagName(step installStep) string {
	switch step {
	case installStepAsk:
		return "ask"
	case installStepLanguage:
		return "language"
	case installStepCurrency:
		return "currency"
	case installStepCredentials:
		return "credentials"
	}
	return "unknown"
}

// migrationWizardTags builds the project.dev.migration_wizard event.
// duration_ms is only present once the user has left the welcome screen
// (startedAt is set on the welcome confirm).
func migrationWizardTags(result string, sg migrationWizard) map[string]string {
	tags := map[string]string{tracking.TagResult: result}
	if !sg.startedAt.IsZero() {
		tags[tracking.TagDurationMS] = durationMS(time.Since(sg.startedAt))
	}
	switch result {
	case tracking.ResultCancelled:
		tags[tracking.TagAbandonedAt] = migrationStepTagName(sg.step)
	case tracking.ResultCompleted, tracking.ResultFailed:
		if sg.phpCursor >= 0 && sg.phpCursor < len(sg.phpVersions) {
			tags[tracking.TagPHPVersion] = sg.phpVersions[sg.phpCursor]
		}
	}
	if result == tracking.ResultCompleted {
		tags[tracking.TagDeploymentHelperAdded] = strconv.FormatBool(sg.deploymentHelperAdded)
	}
	return tags
}

func migrationStepTagName(step migrationStep) string {
	switch step {
	case migrationStepWelcome:
		return "welcome"
	case migrationStepAdminUser:
		return "admin_user"
	case migrationStepDockerPHP:
		return "docker_php"
	case migrationStepReview:
		return "review"
	case migrationStepDone:
		return "done"
	}
	return "unknown"
}

func (t *telemetryState) beginDockerStart() {
	if t == nil {
		return
	}
	t.dockerStart = time.Now()
}

func (t *telemetryState) dockerStartTags(err error) (map[string]string, bool) {
	if t == nil || t.dockerStart.IsZero() {
		return nil, false
	}
	started := t.dockerStart
	t.dockerStart = time.Time{}
	return map[string]string{
		tracking.TagTrigger:    "initial",
		tracking.TagResult:     resultTag(err),
		tracking.TagDurationMS: durationMS(time.Since(started)),
	}, true
}

func (t *telemetryState) beginConfigRestart() {
	if t == nil {
		return
	}
	t.restartStart = time.Now()
}

func (t *telemetryState) configRestartTags(err error) (map[string]string, bool) {
	if t == nil || t.restartStart.IsZero() {
		return nil, false
	}
	started := t.restartStart
	t.restartStart = time.Time{}
	return map[string]string{
		tracking.TagTrigger:    "config_change",
		tracking.TagResult:     resultTag(err),
		tracking.TagDurationMS: durationMS(time.Since(started)),
	}, true
}

func (t *telemetryState) beginTask(action string) {
	if t == nil {
		return
	}
	t.actionCount++
	t.taskAction = action
	t.taskStart = time.Now()
}

func (t *telemetryState) taskTags(result string) (map[string]string, bool) {
	if t == nil || t.taskAction == "" {
		return nil, false
	}
	tags := map[string]string{
		tracking.TagAction:     t.taskAction,
		tracking.TagResult:     result,
		tracking.TagDurationMS: durationMS(time.Since(t.taskStart)),
	}
	t.taskAction = ""
	return tags, true
}

func (t *telemetryState) watcherStarted(name string) {
	if t == nil {
		return
	}
	if _, ok := t.watcherStarts[name]; ok {
		return
	}
	t.watcherStarts[name] = time.Now()
	t.watchersUsed[watcherTagName(name)] = struct{}{}
}

// watcherEndTags builds the project.dev.watcher event for one watcher run and
// forgets its start time, so follow-up messages for the same run (a
// logDoneMsg after a user stop, a watcherRunningMsg error after a stop during
// preparation) don't produce a second event.
func (t *telemetryState) watcherEndTags(name, result string) (map[string]string, bool) {
	if t == nil {
		return nil, false
	}
	started, ok := t.watcherStarts[name]
	if !ok {
		return nil, false
	}
	delete(t.watcherStarts, name)
	return map[string]string{
		tracking.TagWatcher:    watcherTagName(name),
		tracking.TagResult:     result,
		tracking.TagDurationMS: durationMS(time.Since(started)),
	}, true
}

// installFailedStep names the last deployment-helper step that had started
// when the install failed. Failures before the first recognized step report
// the first step.
func installFailedStep(currentStep int) string {
	if currentStep >= len(installStepPatterns) {
		currentStep = len(installStepPatterns) - 1
	}
	return installStepPatterns[currentStep].pattern
}

func watcherTagName(name string) string {
	if name == watcherStorefront {
		return "storefront"
	}
	return "admin"
}

// healthOnce reports whether the health event should be sent and latches, so
// re-running the checks (e.g. after a config-change restart) doesn't send a
// second snapshot per session.
func (t *telemetryState) healthOnce() bool {
	if t == nil || t.healthSent {
		return false
	}
	t.healthSent = true
	return true
}

// healthEventTags turns the setup-health report into one event per check,
// tagged with the check name ("PHP version" → php_version) and its level as
// the result. Reusing the shared check/result keys keeps the events
// aggregatable in ClickHouse without per-check schema knowledge.
func healthEventTags(checks []healthCheck) []map[string]string {
	events := make([]map[string]string, 0, len(checks))
	for _, c := range checks {
		events = append(events, map[string]string{
			tracking.TagCheck:  strings.ReplaceAll(strings.ToLower(c.Name), " ", "_"),
			tracking.TagResult: c.Level.tagValue(),
		})
	}
	return events
}

func (l healthLevel) tagValue() string {
	switch l {
	case healthOK:
		return "ok"
	case healthWarn:
		return "warn"
	case healthCritical:
		return "critical"
	default:
		return "ok"
	}
}

func resultTag(err error) string {
	if err != nil {
		return tracking.ResultFailure
	}
	return tracking.ResultSuccess
}

func durationMS(d time.Duration) string {
	return strconv.FormatInt(d.Milliseconds(), 10)
}

func joinSet(set map[string]struct{}) string {
	values := make([]string, 0, len(set))
	for v := range set {
		values = append(values, v)
	}
	slices.Sort(values)
	return strings.Join(values, ",")
}

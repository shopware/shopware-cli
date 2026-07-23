package upgradetui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: string(code)})
}

func specialKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
}

func ctrlC() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
}

// wizard bundles the hosted app with the model for state assertions.
type wizard struct {
	*app.Harness
	m *Model
}

func newTestWizard(t *testing.T) *wizard {
	t.Helper()
	shell, m := newAppWithModel(Options{ProjectRoot: "/projects/acme-shop", EnvName: "local"})
	h := &app.Harness{App: shell}
	h.Send(tea.WindowSizeMsg{Width: 110, Height: 34})
	return &wizard{Harness: h, m: m}
}

func (w *wizard) view(t *testing.T) string {
	t.Helper()
	return ansi.Strip(w.View())
}

func testReadiness(blocked bool) upgrade.Readiness {
	state := upgrade.StateOK
	if blocked {
		state = upgrade.StateFail
	}
	return upgrade.Readiness{
		CurrentVersion: version.Must(version.NewVersion("6.6.10.3")),
		Checks: []upgrade.ReadinessCheck{
			{ID: "repository", Label: "Repository", Value: "acme-shop", State: upgrade.StateOK},
			{ID: "git-clean", Label: "Git working tree clean", Value: "yes", State: state, Blocking: true},
		},
		Extensions: []upgrade.InstalledExtension{
			{Name: "SwagDemo", Package: "swag/demo", Version: "2.0.0", ComposerManaged: true},
		},
	}
}

func testCatalog() *upgrade.Catalog {
	return &upgrade.Catalog{
		Current: version.Must(version.NewVersion("6.6.10.3")),
		Options: []upgrade.VersionOption{
			{Version: version.Must(version.NewVersion("6.7.11.0")), Tag: "recommended", SupportType: "active"},
			{Version: version.Must(version.NewVersion("6.7.10.0")), SupportType: "active"},
			{Version: version.Must(version.NewVersion("6.6.10.19")), Tag: "latest 6.6 patch", SupportType: "security"},
		},
		Recommended: 0,
		LatestPatch: 2,
	}
}

// wizardAtCheck returns a wizard on panel 2 with checks and catalog loaded.
func wizardAtCheck(t *testing.T, blocked bool) *wizard {
	t.Helper()
	w := newTestWizard(t)
	w.Send(specialKey(tea.KeyEnter)) // Begin upgrade
	w.Send(checksDoneMsg{readiness: testReadiness(blocked)})
	w.Send(catalogLoadedMsg{catalog: testCatalog()})
	return w
}

func TestIntroPanel(t *testing.T) {
	w := newTestWizard(t)

	content := w.view(t)
	assert.Contains(t, content, "Upgrade Shopware to a newer version")
	assert.Contains(t, content, "Check project readiness")
	assert.Contains(t, content, "Begin upgrade")
	assert.Contains(t, content, "acme-shop")
	assert.Contains(t, content, "local")

	cmd := w.Send(specialKey(tea.KeyEnter))
	assert.Equal(t, panelCheck, w.m.panel)
	assert.NotNil(t, cmd, "entering the check panel starts the readiness checks")

	assert.Equal(t, "Upgrade · acme-shop", w.App.View().WindowTitle)
}

func TestIntroCancelQuits(t *testing.T) {
	w := newTestWizard(t)
	w.Send(specialKey(tea.KeyRight))
	cmd := w.Send(specialKey(tea.KeyEnter))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd(), "Cancel quits the program")
}

func TestCtrlCQuitsOutsideRunPanel(t *testing.T) {
	w := newTestWizard(t)
	cmd := w.Send(ctrlC())
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestCheckPanelReady(t *testing.T) {
	w := wizardAtCheck(t, false)

	content := w.view(t)
	assert.Contains(t, content, "Check project + choose Shopware version")
	assert.Contains(t, content, "Git working tree clean")
	assert.Contains(t, content, "Project is ready. Choose a Shopware version next.")
	assert.Contains(t, content, "6.7.11.0")
	assert.Contains(t, content, "recommended")
	assert.Contains(t, content, "latest 6.6 patch")
	assert.Contains(t, content, "Choose another supported version…")
	assert.Contains(t, content, "Shopware 6.6.10.3", "header shows the current version")

	require.NotNil(t, w.m.check.target())
	assert.Equal(t, "6.7.11.0", w.m.check.target().Version.String(), "recommended is preselected")
}

func TestCheckPanelBlocked(t *testing.T) {
	w := wizardAtCheck(t, true)

	content := w.view(t)
	assert.Contains(t, content, "BLOCKED")
	assert.Contains(t, content, "Fix the blocking checks")

	// Continue must not advance while blocked.
	w.Send(specialKey(tea.KeyEnter))
	assert.Equal(t, panelCheck, w.m.panel)
}

func TestCheckPanelRecheck(t *testing.T) {
	w := wizardAtCheck(t, true)

	cmd := w.Send(key('r'))
	assert.True(t, w.m.check.loading)
	assert.NotNil(t, cmd)
}

func TestVersionPickerOverlay(t *testing.T) {
	w := wizardAtCheck(t, false)

	// Move the cursor to "Choose another supported version…" and open it.
	w.Send(specialKey(tea.KeyDown), specialKey(tea.KeyDown))
	w.Send(specialKey(tea.KeyEnter))
	require.True(t, w.App.OverlayOpen())

	content := w.view(t)
	assert.Contains(t, content, "Select a supported Shopware version")
	assert.Contains(t, content, "Current project: Shopware 6.6.10.3")
	assert.Contains(t, content, "6.7.10.0")
	assert.Contains(t, content, "Shopware CLI", "branding header stays visible behind the overlay")

	// Choose the second entry. Deliver the picker's result message directly —
	// resolving further commands would execute the real preparation batch.
	w.Send(specialKey(tea.KeyDown))
	cmd := w.Send(specialKey(tea.KeyEnter))
	require.NotNil(t, cmd)
	w.Send(cmd())

	assert.False(t, w.App.OverlayOpen())
	require.NotNil(t, w.m.check.target())
	assert.Equal(t, "6.7.10.0", w.m.check.target().Version.String())
	assert.Equal(t, panelPrepare, w.m.panel, "confirming in the picker continues to the prepare panel")
}

func TestVersionPickerEscCloses(t *testing.T) {
	w := wizardAtCheck(t, false)
	w.Send(specialKey(tea.KeyDown), specialKey(tea.KeyDown))
	w.Send(specialKey(tea.KeyEnter))
	require.True(t, w.App.OverlayOpen())

	cmd := w.Send(specialKey(tea.KeyEscape))
	require.NotNil(t, cmd)
	w.SendCmd(cmd)
	assert.False(t, w.App.OverlayOpen())
	// Navigation and the dismissed picker never touched the selection.
	require.NotNil(t, w.m.check.target())
	assert.Equal(t, "6.7.11.0", w.m.check.target().Version.String(), "closing without picking keeps the previous selection")
}

func TestCheckPanelCursorDoesNotChangeSelection(t *testing.T) {
	w := wizardAtCheck(t, false)

	// Navigate across all rows: the ◉ stays on the recommended version until
	// a row is activated with Enter.
	w.Send(specialKey(tea.KeyDown), specialKey(tea.KeyDown))
	require.NotNil(t, w.m.check.target())
	assert.Equal(t, "6.7.11.0", w.m.check.target().Version.String())

	content := w.view(t)
	assert.Contains(t, content, "◉ 6.7.11.0")
	assert.Contains(t, content, "> ○ Choose another supported version…")

	// Enter on a version row selects it (and continues).
	w.Send(specialKey(tea.KeyUp))
	w.Send(specialKey(tea.KeyEnter))
	assert.Equal(t, "6.6.10.19", w.m.check.target().Version.String())
	assert.Equal(t, panelPrepare, w.m.panel)
}

// wizardAtPrepare returns a wizard on panel 3 with the given extension results.
func wizardAtPrepare(t *testing.T, results []upgrade.ExtensionResult, resolveOK bool) *wizard {
	t.Helper()
	w := wizardAtCheck(t, false)
	w.Send(specialKey(tea.KeyEnter)) // Continue
	require.Equal(t, panelPrepare, w.m.panel)

	gen := w.m.prepareGen
	w.Send(envStatusMsg{gen: gen, running: true})
	w.Send(packagistMsg{gen: gen, reachable: true})
	w.Send(resolveDoneMsg{gen: gen, result: upgrade.ResolveResult{OK: resolveOK, Report: "report"}})
	w.Send(compatDoneMsg{gen: gen, results: results})
	w.Send(phpInfoMsg{gen: gen, requirement: ">=8.2", installed: "8.3.1"})
	return w
}

func TestPrepareIgnoresSupersededResults(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{okResult()}, true)
	staleGen := w.m.prepareGen

	// Back out and start a new preparation run: results still in flight from
	// the first run must not populate the new state.
	w.Send(specialKey(tea.KeyEscape))
	require.Equal(t, panelCheck, w.m.panel)
	w.Send(specialKey(tea.KeyEnter)) // Continue again
	require.Equal(t, panelPrepare, w.m.panel)
	require.NotEqual(t, staleGen, w.m.prepareGen)

	w.Send(compatDoneMsg{gen: staleGen, results: []upgrade.ExtensionResult{blockedResult()}})
	assert.False(t, w.m.prepare.compatDone, "stale compat results are dropped")
	assert.Empty(t, w.m.prepare.results)

	w.Send(resolveDoneMsg{gen: staleGen, result: upgrade.ResolveResult{OK: true}})
	assert.Nil(t, w.m.prepare.resolve, "stale resolve results are dropped")
}

func okResult() upgrade.ExtensionResult {
	return upgrade.ExtensionResult{
		Extension: upgrade.InstalledExtension{Name: "SwagDemo", Package: "swag/demo", Version: "2.0.0", ComposerManaged: true},
		Status:    upgrade.ExtOK,
		Available: "2.0.0",
	}
}

func blockedResult() upgrade.ExtensionResult {
	return upgrade.ExtensionResult{
		Extension: upgrade.InstalledExtension{Name: "AcmeERPConnector", Package: "acme/erp", Version: "3.2.0", ComposerManaged: true},
		Status:    upgrade.ExtBlocked,
		Detail:    "No released version of this extension is compatible.",
	}
}

func TestPreparePanelReady(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{okResult()}, true)

	content := w.view(t)
	assert.Contains(t, content, "Prepare upgrade")
	assert.Contains(t, content, "6.6.10.3 → 6.7.11.0", "title bar shows the version path")
	assert.Contains(t, content, "READY")
	assert.Contains(t, content, "Packagist reachable")
	assert.Contains(t, content, "Composer can resolve this upgrade")
	assert.Contains(t, content, "SwagDemo")
	assert.Contains(t, content, "2.0.0 -> 2.0.0")

	w.Send(key('c'))
	assert.Equal(t, panelReview, w.m.panel, "continue is allowed when ready")
}

func TestPreparePanelEnterOnContinueButton(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{okResult()}, true)

	// Moving past the last extension row focuses the Continue button.
	w.Send(specialKey(tea.KeyDown))
	assert.Equal(t, 1, w.m.prepare.cursor)
	w.Send(specialKey(tea.KeyEnter))
	assert.Equal(t, panelReview, w.m.panel, "enter on the focused Continue button continues")
}

func TestPreparePanelLeftRightFocusesButton(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{blockedResult(), okResult()}, true)
	w.Send(specialKey(tea.KeyDown)) // second queue row
	assert.Equal(t, 1, w.m.prepare.cursor)

	// Right jumps to the Continue button in the right column.
	w.Send(specialKey(tea.KeyRight))
	assert.Equal(t, 2, w.m.prepare.cursor, "right focuses the Continue button")

	// Left returns to the previously selected queue row.
	w.Send(specialKey(tea.KeyLeft))
	assert.Equal(t, 1, w.m.prepare.cursor, "left restores the queue selection")

	w.Send(specialKey(tea.KeyRight))
	w.Send(specialKey(tea.KeyEnter))
	assert.Equal(t, panelReview, w.m.panel, "enter on the right-focused button continues")
}

func TestPreparePanelEmptyQueueEnterContinues(t *testing.T) {
	w := wizardAtPrepare(t, nil, true)

	content := w.view(t)
	assert.Contains(t, content, "No extensions found.")
	assert.Contains(t, content, "> ", "the Continue button is focused from the start")

	w.Send(specialKey(tea.KeyEnter))
	assert.Equal(t, panelReview, w.m.panel, "enter continues immediately when there is no queue")
}

func TestPreparePanelEnterDoesNotContinueWhenBlocked(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{blockedResult()}, false)

	w.Send(specialKey(tea.KeyDown))
	w.Send(specialKey(tea.KeyEnter))
	assert.Equal(t, panelPrepare, w.m.panel, "a focused but unready Continue button does nothing")
}

func TestPrepareResolveFailureShowsConflictInline(t *testing.T) {
	w := wizardAtCheck(t, false)
	w.Send(specialKey(tea.KeyEnter))
	require.Equal(t, panelPrepare, w.m.panel)

	gen := w.m.prepareGen
	w.Send(envStatusMsg{gen: gen, running: true})
	w.Send(packagistMsg{gen: gen, reachable: true})
	w.Send(compatDoneMsg{gen: gen, results: []upgrade.ExtensionResult{okResult()}})
	cmd := w.Send(resolveDoneMsg{gen: gen, result: upgrade.ResolveResult{
		OK:     false,
		Report: "Loading composer repositories\nProblem 1\n    - shopware/core 6.7.11.0 conflicts with swag/demo 2.0.0",
	}})
	w.Send(phpInfoMsg{gen: gen, requirement: ">=8.2", installed: "8.3.1"})

	require.NotNil(t, cmd, "a failed resolution writes the report immediately")

	// The solver's conflict summary replaces the extension queue.
	content := w.view(t)
	assert.Contains(t, content, "Composer conflict")
	assert.Contains(t, content, "conflicts with swag/demo")
	assert.NotContains(t, content, "Extension queue")

	// Once the report is written, its location is surfaced too (the status
	// strip may truncate the path to the frame width).
	w.Send(reportWrittenMsg{path: "/projects/acme-shop/.shopware-cli/upgrade/report.md"})
	content = w.view(t)
	assert.Contains(t, content, "Report: .shopware-cli")
	assert.Contains(t, content, "Full output: .shopware-cli/upgrade/report.md")
}

func TestPreparePanelBlocked(t *testing.T) {
	// Flagged extensions AND a failed composer resolution: blocked.
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{blockedResult(), okResult()}, false)

	content := w.view(t)
	assert.Contains(t, content, "BLOCKED")
	assert.Contains(t, content, "1 extensions need fixes")
	assert.Contains(t, content, "AcmeERPConnector")
	assert.Contains(t, content, "3.2.0 -> none")

	w.Send(key('c'))
	assert.Equal(t, panelPrepare, w.m.panel, "an unresolvable upgrade cannot start")
}

func TestPreparePanelShowsResolvedVersions(t *testing.T) {
	w := wizardAtCheck(t, false)
	w.Send(specialKey(tea.KeyEnter))
	require.Equal(t, panelPrepare, w.m.panel)

	gen := w.m.prepareGen
	w.Send(envStatusMsg{gen: gen, running: true})
	w.Send(packagistMsg{gen: gen, reachable: true})
	w.Send(compatDoneMsg{gen: gen, results: []upgrade.ExtensionResult{okResult()}})
	w.Send(resolveDoneMsg{gen: gen, result: upgrade.ResolveResult{
		OK: true,
		Changes: []upgrade.PackageChange{
			{Name: "swag/demo", From: "2.0.0", To: "2.1.3", Op: "upgrade"},
		},
	}})
	w.Send(phpInfoMsg{gen: gen, requirement: ">=8.2", installed: "8.3.1"})

	content := w.view(t)
	assert.Contains(t, content, "2.0.0 -> 2.1.3", "queue shows the version composer resolved to")
}

func TestPreparePanelFlaggedButResolvable(t *testing.T) {
	// Store metadata flags an extension, but composer resolves the upgrade
	// with extensions passed as "*": the solver's verdict wins.
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{blockedResult(), okResult()}, true)

	content := w.view(t)
	assert.Contains(t, content, "REVIEW")
	assert.Contains(t, content, "Composer resolved the upgrade")
	assert.NotContains(t, content, "BLOCKED")

	w.Send(key('c'))
	assert.Equal(t, panelReview, w.m.panel, "flagged extensions alone do not block")
}

func TestPreparePanelComposerBlocked(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{okResult()}, false)

	content := w.view(t)
	assert.Contains(t, content, "BLOCKED")
	assert.Contains(t, content, "Composer cannot resolve this upgrade")

	w.Send(key('c'))
	assert.Equal(t, panelPrepare, w.m.panel)
}

func TestExtensionDetailOverlay(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{blockedResult(), okResult()}, true)

	w.Send(specialKey(tea.KeyEnter))
	require.True(t, w.App.OverlayOpen())
	content := w.view(t)
	assert.Contains(t, content, "Blocked extension")
	assert.Contains(t, content, "AcmeERPConnector")
	assert.Contains(t, content, "Ask the vendor for a compatible release")

	cmd := w.Send(specialKey(tea.KeyEscape))
	require.NotNil(t, cmd)
	w.SendCmd(cmd)
	assert.False(t, w.App.OverlayOpen())
}

func TestExtensionDetailVariants(t *testing.T) {
	cases := []struct {
		status upgrade.ExtStatus
		badge  string
	}{
		{upgrade.ExtOK, "OK"},
		{upgrade.ExtNeedsUpdate, "NEEDS UPDATE"},
		{upgrade.ExtMismatch, "NEEDS REVIEW"},
		{upgrade.ExtDeprecated, "REPLACE REQUIRED"},
		{upgrade.ExtBlocked, "BLOCKED"},
		{upgrade.ExtReview, "REVIEW"},
	}

	for _, tc := range cases {
		result := okResult()
		result.Status = tc.status
		detail := newExtensionDetail(result, "Target 6.7.11.0")
		content := ansi.Strip(detail.View(110, 34))
		assert.Contains(t, content, tc.badge, "status %v renders its badge", tc.status)
		assert.Contains(t, content, "User action")
	}
}

func TestReviewPanel(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{okResult()}, true)
	w.Send(key('c'))
	require.Equal(t, panelReview, w.m.panel)

	content := w.view(t)
	assert.Contains(t, content, "Review upgrade plan and start")
	assert.Contains(t, content, "composer update --with-all-dependencies")
	assert.Contains(t, content, "write .shopware-cli/upgrade/report.md")
	assert.Contains(t, content, "1 compatible extensions")
	assert.Contains(t, content, "Start upgrade")
	assert.Contains(t, content, "Export report")

	data := w.m.reportData()
	assert.Equal(t, "acme-shop", data.ProjectName)
	assert.Equal(t, "6.6.10.3", data.Current)
	assert.Equal(t, "6.7.11.0", data.Target)
	assert.Equal(t, ">=8.2", data.PHPRequirement)
	assert.Empty(t, data.ComposerReport, "no composer report when resolution succeeded")
}

func TestReviewBackReturnsToPrepare(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{okResult()}, true)
	w.Send(key('c'))

	w.Send(specialKey(tea.KeyEscape))
	assert.Equal(t, panelPrepare, w.m.panel)
}

func TestRunPanelProgress(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{okResult()}, true)
	w.Send(key('c'))

	// Enter the run panel without starting a real runner.
	w.m.panel = panelRun
	w.m.run = runState{
		stepStates: make(map[upgrade.StepID]upgrade.CheckState),
		stepErrs:   make(map[upgrade.StepID]error),
	}

	w.Send(runEventMsg(upgrade.StepEvent{Step: upgrade.StepComposerUpdate, State: upgrade.StateRunning}))
	w.Send(runEventMsg(upgrade.StepEvent{Step: upgrade.StepComposerUpdate, State: upgrade.StateRunning, Line: "Updating dependencies"}))

	content := w.view(t)
	assert.Contains(t, content, "Upgrade in progress")
	assert.Contains(t, content, "RUNNING")
	assert.Contains(t, content, "composer update --with-all-dependencies")
	assert.Contains(t, content, "Updating dependencies")

	// Full-width log toggle.
	w.Send(key('l'))
	assert.True(t, w.m.run.fullLog)
	assert.Contains(t, w.view(t), "Updating dependencies")

	// Ctrl+c cancels instead of quitting while the run is unfinished.
	cmd := w.Send(ctrlC())
	assert.Nil(t, cmd, "quit is swallowed while the upgrade runs")

	// Successful finish moves to the done panel once the stream closes.
	w.Send(runEventMsg(upgrade.StepEvent{Step: upgrade.StepFinished, State: upgrade.StateOK}))
	w.Send(runClosedMsg{})
	assert.Equal(t, panelDone, w.m.panel)
	assert.True(t, w.m.done.succeeded)
}

func TestDonePanelSuccess(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{okResult()}, true)
	w.m.panel = panelDone
	w.m.done = doneState{succeeded: true}

	content := w.view(t)
	assert.Contains(t, content, "Upgrade report")
	assert.Contains(t, content, "DONE")
	assert.Contains(t, content, "Shopware packages updated")
	assert.Contains(t, content, "6.6.10.3 -> 6.7.11.0")
	assert.Contains(t, content, "Commit composer.json and composer.lock")
	assert.Contains(t, content, "Shopware 6.7.11.0", "header shows the new version")

	cmd := w.Send(specialKey(tea.KeyEnter))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestDonePanelFailure(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{okResult()}, true)
	w.m.panel = panelDone
	w.m.done = doneState{succeeded: false, err: assert.AnError}

	content := w.view(t)
	assert.Contains(t, content, "Upgrade failed")
	assert.Contains(t, content, "FAILED")
	assert.Contains(t, content, "composer.json and composer.lock were restored")
	assert.Contains(t, content, "Run composer install to restore vendor/")
}

func TestEveryPanelFitsTheWindow(t *testing.T) {
	w := wizardAtPrepare(t, []upgrade.ExtensionResult{blockedResult(), okResult()}, true)

	panels := []panel{panelIntro, panelCheck, panelPrepare, panelReview, panelRun, panelDone}
	w.m.run.stepStates = make(map[upgrade.StepID]upgrade.CheckState)
	w.m.run.stepErrs = make(map[upgrade.StepID]error)

	for _, p := range panels {
		w.m.panel = p
		lines := strings.Split(w.View(), "\n")
		assert.Len(t, lines, 34, "panel %d fills the window height exactly", p)
	}
}

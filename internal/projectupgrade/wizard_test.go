package projectupgrade

import (
	"os"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/tui"
)

func newTestModel(t *testing.T) wizardModel {
	t.Helper()
	return newTestModelWithVersions(t, []string{"6.6.4.0", "6.6.3.0", "6.5.9.0"})
}

func newTestModelWithVersions(t *testing.T, versions []string) wizardModel {
	t.Helper()

	current, err := version.NewVersion("6.5.8.0")
	require.NoError(t, err)

	opts := make([]tui.SelectOption, len(versions))
	for i, v := range versions {
		opts[i] = tui.SelectOption{Label: v}
	}

	m := wizardModel{
		opts: WizardOptions{
			ProjectRoot:      "/tmp/example",
			ComposerJSONPath: "/tmp/example/composer.json",
			CurrentVersion:   current,
			UpdateVersions:   versions,
		},
		phase:        phaseWelcome,
		confirmYes:   true,
		versionList:  tui.NewSelectList("Select target version", "", opts, maxVisibleVersions),
		tasks:        defaultTasks(),
		markedRemove: map[string]bool{},
	}
	return m
}

func keyPress(c rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: c, Text: string(c)}
}

func TestWizardWelcomeConfirmGoesToPreflight(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wm := updated.(wizardModel)
	assert.Equal(t, phasePreflight, wm.phase)
	assert.True(t, wm.preflightLoading)
	assert.NotNil(t, cmd, "entering preflight must schedule the checks")
}

func TestWizardWelcomeCancelQuits(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.confirmYes = false
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(tea.QuitMsg)
	assert.True(t, ok, "cancel should produce QuitMsg")
}

func TestWizardPreflightBlocksUntilChecksPass(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phasePreflight
	m.preflightLoading = true

	updated, _ := m.Update(preflightDoneMsg{results: []PreflightResult{
		{Label: "Git working tree clean", Status: PreflightFailed, Explanation: "commit your changes"},
	}})
	wm := updated.(wizardModel)
	assert.False(t, wm.preflightLoading)

	// Enter must not advance while a check fails.
	updated, _ = wm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wm = updated.(wizardModel)
	assert.Equal(t, phasePreflight, wm.phase)

	// The failure and its explanation are visible.
	out := wm.viewContent()
	assert.Contains(t, out, "commit your changes")

	// A recheck that passes unblocks the flow.
	updated, _ = wm.Update(keyPress('r'))
	wm = updated.(wizardModel)
	assert.True(t, wm.preflightLoading, "r must rerun the checks")

	updated, _ = wm.Update(preflightDoneMsg{results: []PreflightResult{
		{Label: "Git working tree clean", Status: PreflightOK},
	}})
	wm = updated.(wizardModel)
	updated, _ = wm.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wm = updated.(wizardModel)
	assert.Equal(t, phaseSelectVersion, wm.phase)
}

// Navigation/paging is owned and tested by tui.SelectList; here we only verify
// the wizard forwards keys to it.
func TestWizardSelectVersionForwardsNavigationToList(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseSelectVersion

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	wm := updated.(wizardModel)
	assert.Equal(t, 1, wm.versionList.Cursor())
}

func TestWizardSelectVersionGoesToPrepare(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseSelectVersion

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wm := updated.(wizardModel)
	assert.Equal(t, phasePrepare, wm.phase)
	assert.True(t, wm.compatLoading)
	assert.Equal(t, "6.6.4.0", wm.targetVersion)
}

func TestWizardSelectVersionShowsReleaseNotesLink(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseSelectVersion

	out := m.viewContent()
	assert.Contains(t, out, "releases/tag/v6.6.4.0")
}

func TestWizardPrepareBlockedPreventsContinue(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phasePrepare
	m.compatLoading = true

	updated, _ := m.Update(compatLoadedMsg{
		report: CompatReport{OK: false, BlockingPlugins: []string{"swag/blocked"}},
		queue: []ExtensionRow{
			{Name: "swag/blocked", Current: "1.0.0", State: ExtensionBlocked, Result: "no compatible release"},
			{Name: "swag/ok", Current: "1.0.0", Target: "1.0.0", State: ExtensionOK},
		},
	})
	wm := updated.(wizardModel)
	assert.False(t, wm.compatLoading)
	assert.True(t, wm.prepareBlocked())

	out := wm.viewContent()
	assert.Contains(t, out, "BLOCKED")
	assert.Contains(t, out, "1/2 extensions need fixes")

	// c must not advance while blocked.
	updated, _ = wm.Update(keyPress('c'))
	wm = updated.(wizardModel)
	assert.Equal(t, phasePrepare, wm.phase)
}

func TestWizardPrepareReadyContinuesToReview(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phasePrepare
	m.targetVersion = "6.6.4.0"

	updated, _ := m.Update(compatLoadedMsg{
		report: CompatReport{OK: true},
		queue: []ExtensionRow{
			{Name: "swag/ok", Current: "1.0.0", Target: "1.0.0", State: ExtensionOK},
		},
	})
	wm := updated.(wizardModel)

	out := wm.viewContent()
	assert.Contains(t, out, "READY")

	updated, _ = wm.Update(keyPress('c'))
	wm = updated.(wizardModel)
	assert.Equal(t, phaseReview, wm.phase)
}

func TestWizardPrepareOverlayShowsDetailsAndMarksRemoval(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phasePrepare
	m.targetVersion = "6.6.4.0"
	m.extQueue = []ExtensionRow{
		{Name: "swag/blocked", Current: "1.0.0", State: ExtensionBlocked, Result: "no compatible release"},
	}

	// Enter opens the detail overlay.
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wm := updated.(wizardModel)
	assert.True(t, wm.overlayOpen)

	out := wm.viewContent()
	assert.Contains(t, out, "swag/blocked")
	assert.Contains(t, out, "User action")

	// d marks the blocked extension for removal, which unblocks the queue.
	updated, _ = wm.Update(keyPress('d'))
	wm = updated.(wizardModel)
	assert.Equal(t, ExtensionRemove, wm.extQueue[0].State)
	assert.False(t, wm.prepareBlocked())
	assert.True(t, wm.markedRemove["swag/blocked"])

	// d again undoes the decision.
	updated, _ = wm.Update(keyPress('d'))
	wm = updated.(wizardModel)
	assert.Equal(t, ExtensionBlocked, wm.extQueue[0].State)
	assert.False(t, wm.markedRemove["swag/blocked"])

	// esc closes the overlay.
	updated, _ = wm.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	wm = updated.(wizardModel)
	assert.False(t, wm.overlayOpen)
}

func TestWizardRemovalMarkSurvivesRecheckWhileBlocked(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phasePrepare
	m.markedRemove = map[string]bool{"swag/blocked": true}

	// Recheck still reports the extension as blocked: the mark is re-applied.
	updated, _ := m.Update(compatLoadedMsg{
		report: CompatReport{OK: false, BlockingPlugins: []string{"swag/blocked"}},
		queue: []ExtensionRow{
			{Name: "swag/blocked", Current: "1.0.0", State: ExtensionBlocked, Result: "no compatible release"},
		},
	})
	wm := updated.(wizardModel)
	assert.Equal(t, ExtensionRemove, wm.extQueue[0].State)

	// A later recheck finds a compatible release: the stale mark is dropped.
	updated, _ = wm.Update(compatLoadedMsg{
		report: CompatReport{OK: true},
		queue: []ExtensionRow{
			{Name: "swag/blocked", Current: "1.0.0", Target: "2.0.0", State: ExtensionUpdate, Result: "will be updated"},
		},
	})
	wm = updated.(wizardModel)
	assert.Equal(t, ExtensionUpdate, wm.extQueue[0].State)
	assert.False(t, wm.markedRemove["swag/blocked"])
}

func TestWizardReviewShowsPlannedRemovals(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseReview
	m.targetVersion = "6.6.4.0"
	m.extQueue = []ExtensionRow{
		{Name: "swag/drop-me", Current: "1.0.0", State: ExtensionRemove, Result: "will be removed during the upgrade"},
	}

	out := m.viewContent()
	assert.Contains(t, out, "swag/drop-me")
	assert.Contains(t, out, "removed from composer.json")
}

func TestWizardReportWrittenShownInView(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseReview
	m.targetVersion = "6.6.4.0"

	updated, _ := m.Update(reportWrittenMsg{path: "/tmp/example/shopware-upgrade-report-6.6.4.0.md"})
	wm := updated.(wizardModel)
	assert.Contains(t, wm.viewContent(), "shopware-upgrade-report-6.6.4.0.md")
}

func TestWizardWriteReportCreatesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	m := newTestModel(t)
	m.opts.ProjectRoot = dir
	m.opts.ComposerJSONPath = filepath.Join(dir, "composer.json")
	m.targetVersion = "6.6.4.0"
	m.extQueue = []ExtensionRow{
		{Name: "swag/ok", Current: "1.0.0", Target: "1.0.0", State: ExtensionOK, Result: "compatible as installed"},
	}
	require.NoError(t, os.WriteFile(m.opts.ComposerJSONPath, []byte(`{"require":{}}`), 0o644))

	msg := m.writeReport()()
	written, ok := msg.(reportWrittenMsg)
	require.True(t, ok)
	require.NoError(t, written.err)

	content, err := os.ReadFile(written.path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "# Shopware Upgrade Report")
	assert.Contains(t, string(content), "swag/ok")
}

func TestUpgradeTaskOrderRunsDeploymentHelperLast(t *testing.T) {
	t.Parallel()

	// composer update must rewrite vendor before shopware-deployment-helper
	// runs the install/update lifecycle that drives system:update:prepare,
	// migrations, system:update:finish and theme compilation.
	assert.Less(t, taskComposerJSON, taskComposerUpdate, "composer.json must be rewritten before composer update")
	assert.Less(t, taskComposerUpdate, taskDeploymentHelper, "deployment-helper runs after composer update")

	tasks := defaultTasks()
	require.Len(t, tasks, taskDeploymentHelper+1)
	assert.Equal(t, "composer update --with-all-dependencies", tasks[taskComposerUpdate].label)
	assert.Equal(t, "vendor/bin/shopware-deployment-helper run", tasks[taskDeploymentHelper].label)
}

func TestWizardTaskCompleteErrorEndsRun(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseRunning
	m.currentTask = taskComposerUpdate

	updated, _ := m.Update(taskCompleteMsg{
		task:   taskComposerUpdate,
		err:    assertErr("exit status 2"),
		output: []string{"Loading composer repositories", "Your requirements could not be resolved"},
	})
	wm := updated.(wizardModel)
	assert.Equal(t, phaseDone, wm.phase)
	assert.True(t, wm.finished)
	require.Error(t, wm.finalErr)
	assert.Equal(t, taskFailed, wm.tasks[taskComposerUpdate].status)

	// The full subprocess output must be retained so the user can see what
	// actually failed instead of just "exit status 2".
	assert.Equal(t, []string{"Loading composer repositories", "Your requirements could not be resolved"}, wm.fullLog)

	out := wm.viewContent()
	assert.Contains(t, out, "Your requirements could not be resolved", "failed step output should be shown on the done screen")
}

func TestWizardDoneSuccessShowsPostUpgradeChecklist(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseDone
	m.finished = true
	m.targetVersion = "6.6.4.0"

	out := m.viewContent()
	assert.Contains(t, out, "Post-upgrade validation checklist")
	assert.Contains(t, out, "storefront")
	assert.Contains(t, out, "test suite")
}

func TestWizardLogLineMsgAppendsAndTrims(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	total := maxLogLines + 5
	for i := 0; i < total; i++ {
		updated, _ := m.Update(logLineMsg("line"))
		m = updated.(wizardModel)
	}
	assert.LessOrEqual(t, len(m.logLines), maxLogLines, "live view stays capped")
	assert.Len(t, m.fullLog, total, "full log keeps every line for failure reporting")
}

func TestLastLines(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"c", "d"}, lastLines([]string{"a", "b", "c", "d"}, 2))
	assert.Equal(t, []string{"a", "b"}, lastLines([]string{"a", "b"}, 5), "fewer than n returns all")
	assert.Nil(t, lastLines(nil, 3))
}

type testError string

func (e testError) Error() string { return string(e) }

func assertErr(s string) error { return testError(s) }

func TestWizardRendersAllPhases(t *testing.T) {
	t.Parallel()

	phases := []phase{
		phaseWelcome,
		phasePreflight,
		phaseSelectVersion,
		phasePrepare,
		phaseReview,
		phaseRunning,
		phaseDone,
	}

	for _, p := range phases {
		t.Run(t.Name(), func(t *testing.T) {
			t.Parallel()
			m := newTestModel(t)
			m.phase = p
			m.targetVersion = "6.6.4.0"
			out := m.viewContent()
			assert.NotEmpty(t, out, "phase %d should render content", p)

			header := m.headerBar()
			assert.Contains(t, header, "Shopware 6.5.8.0", "header shows the current project context")
		})
	}
}

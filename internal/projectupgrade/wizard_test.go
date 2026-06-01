package projectupgrade

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
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
		phase:       phaseWelcome,
		confirmYes:  true,
		versionList: tui.NewSelectList("Select target version", "", opts, maxVisibleVersions),
		tasks:       defaultTasks(),
	}
	return m
}

func TestWizardWelcomeConfirmGoesToVersionSelect(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wm := updated.(wizardModel)
	assert.Equal(t, phaseSelectVersion, wm.phase)
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

func TestWizardSelectVersionWithoutExtensionsSkipsToReview(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseSelectVersion
	m.versionList.HandleKey("down") // move to "6.6.3.0"

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wm := updated.(wizardModel)
	assert.Equal(t, phaseReview, wm.phase)
	assert.Equal(t, "6.6.3.0", wm.targetVersion)
}

func TestWizardSelectVersionWithExtensionsGoesToCompatCheck(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.opts.Extensions = map[string]string{"AcmeExtension": "1.0.0"}
	m.phase = phaseSelectVersion

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	wm := updated.(wizardModel)
	assert.Equal(t, phaseCompatCheck, wm.phase)
	assert.True(t, wm.compatLoading)
	assert.Equal(t, "6.6.4.0", wm.targetVersion)
}

func TestWizardCompatLoadedSetsBlockerFlag(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseCompatCheck
	m.compatLoading = true

	updated, _ := m.Update(compatLoadedMsg{
		updates: []account_api.UpdateCheckExtensionCompatibility{
			{
				Name: "Blocker",
				Status: account_api.UpdateCheckExtensionCompatibilityStatus{
					Name:  account_api.CompatibilityNotCompatible,
					Type:  "red",
					Label: "Not compatible",
				},
			},
		},
	})
	wm := updated.(wizardModel)
	assert.False(t, wm.compatLoading)
	assert.Equal(t, phaseCompatResult, wm.phase)
	assert.True(t, wm.compatHasBlock)
	assert.False(t, wm.confirmYes, "blocker should default the confirm to No")
}

func TestWizardCompatLoadedUpdatableIsNotBlocker(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseCompatCheck
	m.compatLoading = true

	// "With new Shopware version" — a compatible release exists, so this must
	// not block the upgrade; the resolver bumps the constraint.
	updated, _ := m.Update(compatLoadedMsg{
		updates: []account_api.UpdateCheckExtensionCompatibility{
			{
				Name: "SwagPayPal",
				Status: account_api.UpdateCheckExtensionCompatibilityStatus{
					Name:  account_api.CompatibilityUpdatableNow,
					Type:  "yellow",
					Label: "With new Shopware version",
				},
			},
		},
	})
	wm := updated.(wizardModel)
	assert.Equal(t, phaseCompatResult, wm.phase)
	assert.False(t, wm.compatHasBlock, "updatable extension must not block")
	assert.True(t, wm.compatHasUpdatable)
	assert.True(t, wm.confirmYes, "no blocker means confirm defaults to Yes")
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
		phaseSelectVersion,
		phaseCompatCheck,
		phaseCompatResult,
		phaseReview,
		phaseRunning,
		phaseDone,
	}

	for _, p := range phases {
		p := p
		t.Run(t.Name(), func(t *testing.T) {
			t.Parallel()
			m := newTestModel(t)
			m.phase = p
			m.targetVersion = "6.6.4.0"
			out := m.viewContent()
			assert.NotEmpty(t, out, "phase %d should render content", p)
		})
	}
}

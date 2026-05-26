package projectupgrade

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
)

func newTestModel(t *testing.T) wizardModel {
	t.Helper()

	current, err := version.NewVersion("6.5.8.0")
	require.NoError(t, err)

	m := wizardModel{
		opts: WizardOptions{
			ProjectRoot:      "/tmp/example",
			ComposerJSONPath: "/tmp/example/composer.json",
			CurrentVersion:   current,
			UpdateVersions:   []string{"6.6.4.0", "6.6.3.0", "6.5.9.0"},
		},
		phase:      phaseWelcome,
		confirmYes: true,
		tasks:      defaultTasks(),
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

func TestWizardSelectVersionMovesCursor(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseSelectVersion

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	wm := updated.(wizardModel)
	assert.Equal(t, 1, wm.versionCursor)

	updated, _ = wm.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	wm = updated.(wizardModel)
	assert.Equal(t, 2, wm.versionCursor)

	// Past end should not wrap.
	updated, _ = wm.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	wm = updated.(wizardModel)
	assert.Equal(t, 2, wm.versionCursor)

	updated, _ = wm.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	wm = updated.(wizardModel)
	assert.Equal(t, 1, wm.versionCursor)
}

func TestWizardSelectVersionWithoutExtensionsSkipsToReview(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseSelectVersion
	m.versionCursor = 1

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
					Type:  "violation",
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

func TestWizardTaskCompletePersistsBackupAcrossUpdates(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseRunning
	m.currentTask = taskBackup

	// First task: backup captures composer.json bytes.
	updated, _ := m.Update(taskCompleteMsg{
		task:           taskBackup,
		composerBackup: []byte(`{"name":"shopware/production"}`),
		detail:         "30 bytes",
	})
	wm := updated.(wizardModel)
	assert.Equal(t, []byte(`{"name":"shopware/production"}`), wm.composerBackup, "backup must persist for later restore-on-failure")
	assert.Equal(t, taskCleanup, wm.currentTask)
	assert.Equal(t, taskDone, wm.tasks[taskBackup].status)
}

func TestWizardTaskCompleteErrorEndsRun(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	m.phase = phaseRunning
	m.currentTask = taskComposerUpdate

	updated, _ := m.Update(taskCompleteMsg{
		task: taskComposerUpdate,
		err:  assertErr("composer update failed"),
	})
	wm := updated.(wizardModel)
	assert.Equal(t, phaseDone, wm.phase)
	assert.True(t, wm.finished)
	require.Error(t, wm.finalErr)
	assert.Equal(t, taskFailed, wm.tasks[taskComposerUpdate].status)
}

func TestWizardLogLineMsgAppendsAndTrims(t *testing.T) {
	t.Parallel()

	m := newTestModel(t)
	for i := 0; i < maxLogLines+5; i++ {
		updated, _ := m.Update(logLineMsg("line"))
		m = updated.(wizardModel)
	}
	assert.LessOrEqual(t, len(m.logLines), maxLogLines)
}

type stringErr string

func (e stringErr) Error() string { return string(e) }

func assertErr(s string) error { return stringErr(s) }

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
			m := newTestModel(t)
			m.phase = p
			m.targetVersion = "6.6.4.0"
			out := m.viewContent()
			assert.NotEmpty(t, out, "phase %d should render content", p)
		})
	}
}

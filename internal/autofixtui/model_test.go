package autofixtui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop/pluginmigrate"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

func key(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code, Text: string(code)})
}

func specialKey(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: code})
}

// wizard bundles the hosted app with the model for state assertions.
type wizard struct {
	*app.Harness
	m *Model
}

func newTestWizard(t *testing.T) *wizard {
	t.Helper()
	shell, m := newAppWithModel(Options{ProjectRoot: "/projects/acme-shop"})
	h := &app.Harness{App: shell}
	h.Send(tea.WindowSizeMsg{Width: 110, Height: 34})
	return &wizard{Harness: h, m: m}
}

func (w *wizard) view(t *testing.T) string {
	t.Helper()
	return ansi.Strip(w.View())
}

func testScan() []pluginmigrate.ScannedExtension {
	return []pluginmigrate.ScannedExtension{
		{Name: "LocalPlugin", ComposerName: "acme/local-plugin", Version: "1.0.0", RelPath: "custom/plugins/LocalPlugin"},
		{Name: "StorePlugin", ComposerName: "swag/store-plugin", Version: "3.1.0", RelPath: "custom/plugins/StorePlugin"},
	}
}

// wizardAtToken drives the wizard through the welcome confirmation.
func wizardAtToken(t *testing.T) *wizard {
	t.Helper()
	w := newTestWizard(t)
	w.Send(scanDoneMsg{extensions: testScan()})
	w.Send(specialKey(tea.KeyEnter))
	require.Equal(t, panelToken, w.m.panel)
	return w
}

func TestWelcomeShowsScannedExtensions(t *testing.T) {
	w := newTestWizard(t)
	w.Send(scanDoneMsg{extensions: testScan()})

	content := w.view(t)
	assert.Contains(t, content, "Let's get your plugins under Composer control!")
	assert.Contains(t, content, "Found 2 extensions in custom/")
	assert.Contains(t, content, "LocalPlugin")
	assert.Contains(t, content, "StorePlugin")
	assert.Contains(t, content, "Let's fix this")
}

func TestWelcomeIgnoresEnterUntilScanCompletes(t *testing.T) {
	w := newTestWizard(t)
	cmd := w.Send(specialKey(tea.KeyEnter))

	assert.Nil(t, cmd)
	assert.Equal(t, panelWelcome, w.m.panel)
}

func TestWelcomeNothingToDoQuits(t *testing.T) {
	w := newTestWizard(t)
	w.Send(scanDoneMsg{})

	content := w.view(t)
	assert.Contains(t, content, "All extensions are already managed through Composer.")

	cmd := w.Send(specialKey(tea.KeyEnter))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestTokenPanelValidatesAndBuildsStorePlan(t *testing.T) {
	w := wizardAtToken(t)

	content := w.view(t)
	assert.Contains(t, content, "Shopware Packagist token")
	assert.Contains(t, content, "account.shopware.com")

	// Empty token shows a hint instead of continuing.
	w.Send(specialKey(tea.KeyEnter))
	assert.Equal(t, panelToken, w.m.panel)
	assert.Contains(t, w.view(t), "Enter a token, or continue without one")

	// Deliver a fetched Store catalog: the review plan classifies against it.
	for _, r := range "tok" {
		w.Send(key(r))
	}
	w.Send(specialKey(tea.KeyEnter))
	assert.True(t, w.m.tokenBusy)
	w.Send(availabilityMsg{token: "tok", avail: pluginmigrate.Availability{
		Store: map[string]struct{}{"store.shopware.com/storeplugin": {}},
	}})

	require.Equal(t, panelReview, w.m.panel)
	assert.Equal(t, "tok", w.m.token)
	assert.Equal(t, 1, w.m.plan.Count(pluginmigrate.ActionStoreRequire))
	assert.Equal(t, 1, w.m.plan.Count(pluginmigrate.ActionPathRepository))

	content = w.view(t)
	assert.Contains(t, content, "require from Shopware Store")
	assert.Contains(t, content, "manage via path repository")
	assert.Contains(t, content, "add Composer repository")
	assert.Contains(t, content, "store the Packagist token in auth.json")
	assert.Contains(t, content, "Apply")
}

func TestTokenPanelAcceptsPaste(t *testing.T) {
	w := wizardAtToken(t)

	w.Send(tea.PasteMsg{Content: "pasted-token"})
	assert.Equal(t, "pasted-token", w.m.tokenInput.Value())

	w.Send(specialKey(tea.KeyEnter))
	assert.True(t, w.m.tokenBusy, "the pasted token is submitted for validation")
}

func TestTokenPanelFetchErrorStays(t *testing.T) {
	w := wizardAtToken(t)
	for _, r := range "bad" {
		w.Send(key(r))
	}
	w.Send(specialKey(tea.KeyEnter))
	w.Send(availabilityMsg{token: "bad", err: errors.New("401 unauthorized")})

	assert.Equal(t, panelToken, w.m.panel)
	assert.Contains(t, w.view(t), "401 unauthorized")
}

func TestTokenPanelSkipManagesEverythingLocally(t *testing.T) {
	w := wizardAtToken(t)

	// Down focuses the "continue without token" button; enter confirms.
	// Packagist is still checked, so the wizard goes busy first.
	w.Send(specialKey(tea.KeyDown))
	w.Send(specialKey(tea.KeyEnter))
	assert.True(t, w.m.tokenBusy)
	w.Send(availabilityMsg{token: ""})

	require.Equal(t, panelReview, w.m.panel)
	assert.Empty(t, w.m.token)
	assert.Equal(t, 2, w.m.plan.Count(pluginmigrate.ActionPathRepository))
	assert.NotContains(t, w.view(t), "auth.json")
}

func TestTokenPanelSkipRequiresPublishedPlugins(t *testing.T) {
	w := wizardAtToken(t)

	w.Send(specialKey(tea.KeyDown))
	w.Send(specialKey(tea.KeyEnter))
	w.Send(availabilityMsg{token: "", avail: pluginmigrate.Availability{
		Published: map[string][]string{"acme/local-plugin": {"1.0.0"}},
	}})

	require.Equal(t, panelReview, w.m.panel)
	assert.Equal(t, 1, w.m.plan.Count(pluginmigrate.ActionComposerRequire))
	assert.Equal(t, 1, w.m.plan.Count(pluginmigrate.ActionPathRepository))
	assert.Contains(t, w.view(t), "require from Packagist")
}

func TestRunPanelStreamsEventsAndFinishes(t *testing.T) {
	w := wizardAtToken(t)
	w.Send(specialKey(tea.KeyDown))
	w.Send(specialKey(tea.KeyEnter))
	w.Send(availabilityMsg{token: ""}) // review with path-repo plan
	require.Equal(t, panelReview, w.m.panel)

	// Drive the run panel with events directly instead of executing.
	w.m.panel = panelRun
	w.m.run = runState{states: make(map[pluginmigrate.StepID]pluginmigrate.StepState)}

	w.Send(runEventMsg{Step: pluginmigrate.StepComposerRequire, State: pluginmigrate.StateRunning})
	w.Send(runEventMsg{Step: pluginmigrate.StepComposerRequire, State: pluginmigrate.StateRunning, Line: "Installing acme/local-plugin"})

	content := w.view(t)
	assert.Contains(t, content, "Migrating plugins to Composer")
	assert.Contains(t, content, "composer require")
	assert.Contains(t, content, "Installing acme/local-plugin")

	w.Send(runEventMsg{Step: pluginmigrate.StepFinished, State: pluginmigrate.StateOK})
	w.Send(runClosedMsg{})

	require.Equal(t, panelDone, w.m.panel)
	assert.True(t, w.m.done.succeeded)
	content = w.view(t)
	assert.Contains(t, content, "All plugins are Composer-managed now!")
	assert.Contains(t, content, "Commit composer.json, composer.lock, and auth.json")
}

func TestRunCancellationGuardsAndReleasesCancelFunc(t *testing.T) {
	w := newTestWizard(t)
	w.m.panel = panelRun
	w.m.run = runState{}
	assert.NotPanics(t, func() { _ = w.m.handleQuit() })

	cancelled := false
	w.m.run.cancel = func() { cancelled = true }
	w.Send(runClosedMsg{})
	assert.True(t, cancelled)
	assert.Equal(t, panelDone, w.m.panel)
}

func TestSummaryCountsOmitsDanglingSeparatorWithoutDetails(t *testing.T) {
	m := &Model{plan: pluginmigrate.Plan{Extensions: []pluginmigrate.PlannedExtension{
		{Kind: pluginmigrate.ActionKind(99)},
	}}}
	assert.Equal(t, "1 extensions in custom/", m.summaryCounts())
}

func TestDonePanelFailure(t *testing.T) {
	w := newTestWizard(t)
	w.m.panel = panelDone
	w.m.done = doneState{succeeded: false, err: errors.New("composer require: exit 2")}

	content := w.view(t)
	assert.Contains(t, content, "The migration did not complete.")
	assert.Contains(t, content, "were restored")
	assert.Contains(t, content, "composer require: exit 2")

	cmd := w.Send(specialKey(tea.KeyEnter))
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

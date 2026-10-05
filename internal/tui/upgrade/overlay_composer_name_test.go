package upgrade

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	backend "github.com/shopware/shopware-cli/internal/shop/upgrade"
	"github.com/shopware/shopware-cli/internal/tui/app"
)

// newNamelessProjectWizard returns a wizard whose project directory exists but
// has a composer.json without a name, so SetComposerName can run for real.
func newNamelessProjectWizard(t *testing.T) *wizard {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{\n    \"require\": {}\n}\n"), 0o644))

	shell, m := newAppWithModel(t.Context(), Options{ProjectRoot: dir, EnvName: "local"})
	h := &app.Harness{App: shell}
	h.Send(tea.WindowSizeMsg{Width: 110, Height: 34})
	return &wizard{Harness: h, m: m}
}

// testReadinessMissingName is a readiness result whose only failure is the
// missing composer.json package name.
func testReadinessMissingName() backend.Readiness {
	r := testReadiness(false)
	r.Checks = append(r.Checks, backend.ReadinessCheck{
		ID:       "composer-name",
		Label:    "composer.json package name",
		Value:    "missing",
		Detail:   "composer.json has no \"name\" — Composer requires a package name and refuses to run without it.",
		State:    backend.StateFail,
		Blocking: true,
	})
	return r
}

// wizardAtFailedNameCheck returns a wizard on panel 2 whose composer-name
// check failed; the package-name prompt has auto-opened.
func wizardAtFailedNameCheck(t *testing.T) *wizard {
	t.Helper()
	w := newNamelessProjectWizard(t)
	w.Send(specialKey(tea.KeyEnter)) // Begin upgrade
	w.Send(checksDoneMsg{readiness: testReadinessMissingName()})
	return w
}

func TestCheckPanelMissingNameAutoOpensPrompt(t *testing.T) {
	w := wizardAtFailedNameCheck(t)

	require.True(t, w.App.OverlayOpen(), "a failed package-name check asks for the name right away")
	content := w.view(t)
	assert.Contains(t, content, "Set a Composer package name")
	assert.Contains(t, content, backend.SuggestComposerName(w.m.opts.ProjectRoot), "the input is pre-filled with the suggestion")
}

func TestCheckPanelMissingNameConfirmWritesComposerJSON(t *testing.T) {
	w := wizardAtFailedNameCheck(t)
	require.True(t, w.App.OverlayOpen())

	// Accept the pre-filled suggestion.
	cmd := w.Send(specialKey(tea.KeyEnter))
	require.NotNil(t, cmd)
	w.Send(cmd()) // deliver the prompt's result message

	assert.False(t, w.App.OverlayOpen())
	assert.True(t, w.m.check.loading, "confirming the name re-runs the readiness checks")

	content, err := os.ReadFile(filepath.Join(w.m.opts.ProjectRoot, "composer.json"))
	require.NoError(t, err)
	assert.Contains(t, string(content), `"name": "`+backend.SuggestComposerName(w.m.opts.ProjectRoot)+`"`)
}

func TestCheckPanelMissingNameInvalidInputReopensPrompt(t *testing.T) {
	w := wizardAtFailedNameCheck(t)
	require.True(t, w.App.OverlayOpen())

	// Append an invalid character to the pre-filled suggestion.
	w.Send(key(' '))
	w.Send(key('A'))
	cmd := w.Send(specialKey(tea.KeyEnter))
	require.NotNil(t, cmd)
	w.Send(cmd())

	require.True(t, w.App.OverlayOpen(), "an invalid name reopens the prompt")
	assert.Contains(t, w.view(t), "is not a valid Composer package name")

	content, err := os.ReadFile(filepath.Join(w.m.opts.ProjectRoot, "composer.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), `"name"`, "an invalid name must not touch composer.json")
}

func TestCheckPanelMissingNameCancelKeepsCheckFailing(t *testing.T) {
	w := wizardAtFailedNameCheck(t)
	require.True(t, w.App.OverlayOpen())

	cmd := w.Send(specialKey(tea.KeyEscape))
	require.NotNil(t, cmd)
	w.Send(cmd())

	assert.False(t, w.App.OverlayOpen())
	assert.False(t, w.m.check.loading, "cancel does not re-run the checks")
	assert.True(t, w.m.composerNameFailed())

	content, err := os.ReadFile(filepath.Join(w.m.opts.ProjectRoot, "composer.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(content), `"name"`)

	// A manual recheck does not auto-open the prompt a second time…
	w.Send(checksDoneMsg{readiness: testReadinessMissingName()})
	assert.False(t, w.App.OverlayOpen(), "the prompt auto-opens only once per panel visit")

	// …but the failing check keeps offering the fix via "n".
	view := w.view(t)
	assert.Contains(t, view, "Press n to set a package name now.")
	assert.Contains(t, w.App.View().Content, "Set package name", "the footer advertises the fix shortcut")
}

func TestCheckPanelNameKeyOpensPrompt(t *testing.T) {
	w := wizardAtFailedNameCheck(t)

	// Dismiss the auto-opened prompt, then reopen it with "n".
	cmd := w.Send(specialKey(tea.KeyEscape))
	w.SendCmd(cmd)
	require.False(t, w.App.OverlayOpen())

	w.Send(key('n'))
	require.True(t, w.App.OverlayOpen())
	assert.Contains(t, w.view(t), "Set a Composer package name")
}

func TestCheckPanelNameKeyIgnoredWhenNameValid(t *testing.T) {
	w := wizardAtCheck(t, false)
	assert.False(t, w.App.OverlayOpen(), "passing checks never auto-open the prompt")

	w.Send(key('n'))
	assert.False(t, w.App.OverlayOpen(), "n does nothing while the package name is fine")
}

func TestComposerNamePromptPrefillsRejectedValue(t *testing.T) {
	prompt := newComposerNamePrompt("/projects/acme-shop", "acme/rejected", assert.AnError)
	assert.Equal(t, "acme/rejected", prompt.Value())

	view := prompt.View(100, 30)
	stripped := strings.Join(strings.Fields(view), " ")
	assert.Contains(t, stripped, strings.Join(strings.Fields(assert.AnError.Error()), " "))
}

package install

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/state"
	"github.com/shopware/shopware-cli/internal/cliversion"
)

func TestValidateAgent(t *testing.T) {
	require.NoError(t, validateAgent("claude-code"))

	assert.ErrorContains(t, validateAgent(""), "--agent")
	for _, bad := range []string{"*", "claude-code,cursor", "claude code"} {
		assert.ErrorContains(t, validateAgent(bad), "single agent", "should reject %q", bad)
	}
}

func TestSplitNameTag(t *testing.T) {
	n, tag := splitNameTag("shopware-cli")
	assert.Equal(t, "shopware-cli", n)
	assert.Equal(t, "", tag)

	n, tag = splitNameTag("shopware-cli@1.2.3")
	assert.Equal(t, "shopware-cli", n)
	assert.Equal(t, "1.2.3", tag)
}

func TestFindInstall(t *testing.T) {
	f := state.File{Installed: []state.InstalledEntry{
		{Name: "a", Agent: "claude-code", Scope: state.ScopeGlobal, ResolvedRevision: "1"},
	}}

	got, ok := findInstall(f, "a", "claude-code", state.ScopeGlobal)
	require.True(t, ok)
	assert.Equal(t, "1", got.ResolvedRevision)

	_, ok = findInstall(f, "a", "codex", state.ScopeGlobal)
	assert.False(t, ok, "different agent is a different install")
}

// call captures how runSkills was invoked by the function under test.
type call struct {
	calls    int
	lastArgv []string
}

// setup points the install-state at a temp config dir and replaces runSkills with
// a fake. All Add/Remove calls in one test share the config dir, so idempotency
// across calls can be observed.
func setup(t *testing.T) *call {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)                                     // macOS UserConfigDir base (global state)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdgcfg")) // Linux UserConfigDir base (global state)
	t.Setenv("PROJECT_ROOT", tmp)                             // project scope resolves here
	t.Chdir(tmp)

	rec := &call{}
	prev := runSkills
	runSkills = func(_ context.Context, argv []string, _ string, _ io.Writer) error {
		rec.calls++
		rec.lastArgv = argv
		return nil
	}
	t.Cleanup(func() { runSkills = prev })

	return rec
}

func runAdd(t *testing.T, o Options) (Result, error) {
	t.Helper()
	var progress bytes.Buffer

	return Add(t.Context(), o, &progress)
}

func runRemove(t *testing.T, o RemoveOptions) (RemoveResult, error) {
	t.Helper()
	var progress bytes.Buffer

	return Remove(t.Context(), o, &progress)
}

func TestAddDryRunTouchesNothing(t *testing.T) {
	rec := setup(t)

	res, err := runAdd(t, Options{Name: "shopware-cli", Agent: "claude-code", Global: true, DryRun: true})
	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.Equal(t, 0, rec.calls, "dry-run must not run skills")

	st, err := state.Read()
	require.NoError(t, err)
	assert.Empty(t, st.Installed, "dry-run must not write state")
}

func TestAddBundledDefaultBranchWhenNoVersion(t *testing.T) {
	rec := setup(t)
	// The test binary reports the dev version, so a bundled skill with no tag
	// installs from the default branch (bare owner/repo + --skill).
	res, err := runAdd(t, Options{Name: "shopware-cli", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.Equal(t, ActionInstalled, res.Action)
	assert.Equal(t, "", res.ResolvedRevision)

	joined := strings.Join(rec.lastArgv, " ")
	assert.Contains(t, joined, "add shopware/shopware-cli --skill shopware-cli")
	assert.NotContains(t, joined, "/tree/")
}

func TestAddBundledPinsToCLIVersion(t *testing.T) {
	rec := setup(t)
	prev := cliversion.Version
	cliversion.Version = "0.18.3"
	t.Cleanup(func() { cliversion.Version = prev })

	res, err := runAdd(t, Options{Name: "shopware-cli", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.Equal(t, "v0.18.3", res.ResolvedRevision, "bare CLI version is normalized to the v tag")
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "github.com/shopware/shopware-cli/tree/v0.18.3/skills/shopware-cli")
}

func TestAddGitWithTagNormalizesRef(t *testing.T) {
	rec := setup(t)

	// User types the bare version; it is normalized to the v-prefixed tag.
	res, err := runAdd(t, Options{Name: "deployment-helper@0.1.9", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.Equal(t, "v0.1.9", res.ResolvedRevision)
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "github.com/shopware/deployment-helper/tree/v0.1.9/skills/deployment-helper")
}

func TestAddGitNoTagInstallsDefaultBranch(t *testing.T) {
	rec := setup(t)

	res, err := runAdd(t, Options{Name: "deployment-helper", Agent: "claude-code"})
	require.NoError(t, err)
	assert.Equal(t, state.ScopeProject, res.Scope)
	assert.Equal(t, "", res.ResolvedRevision)
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "add shopware/deployment-helper --skill deployment-helper")
}

func TestAddIsIdempotentAndReportsAction(t *testing.T) {
	setup(t)

	res, err := runAdd(t, Options{Name: "deployment-helper@0.1.9", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.Equal(t, ActionInstalled, res.Action)

	res, err = runAdd(t, Options{Name: "deployment-helper@0.1.9", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.Equal(t, ActionUnchanged, res.Action)

	res, err = runAdd(t, Options{Name: "deployment-helper@0.2.0", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.Equal(t, ActionUpdated, res.Action)
	assert.Equal(t, "v0.1.9", res.PreviousRevision)
	assert.Equal(t, "v0.2.0", res.ResolvedRevision)
}

func TestAddProjectScopeWritesToProjectRoot(t *testing.T) {
	setup(t)

	res, err := runAdd(t, Options{Name: "shopware-cli", Agent: "claude-code"})
	require.NoError(t, err)
	assert.Equal(t, state.ScopeProject, res.Scope)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	st, err := state.ReadProject(cwd)
	require.NoError(t, err)
	require.Len(t, st.Installed, 1)

	global, err := state.Read()
	require.NoError(t, err)
	assert.Empty(t, global.Installed, "project install must not touch global state")
}

func TestAddGuards(t *testing.T) {
	rec := setup(t)

	_, err := runAdd(t, Options{Name: "does-not-exist", Agent: "claude-code", Global: true})
	assert.ErrorContains(t, err, "unknown integration")

	_, err = runAdd(t, Options{Name: "shopware-cli", Global: true})
	assert.ErrorContains(t, err, "--agent")

	_, err = runAdd(t, Options{Name: "shopware-cli", Agent: "*", Global: true})
	assert.ErrorContains(t, err, "single agent")

	assert.Equal(t, 0, rec.calls)
}

func TestRemoveRecordedInstall(t *testing.T) {
	rec := setup(t)

	_, err := runAdd(t, Options{Name: "shopware-cli", Agent: "claude-code"})
	require.NoError(t, err)
	require.Equal(t, 1, rec.calls)

	res, err := runRemove(t, RemoveOptions{Name: "shopware-cli", Agent: "claude-code"})
	require.NoError(t, err)
	assert.True(t, res.Removed)
	assert.Equal(t, 2, rec.calls) // add + remove
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "remove shopware-cli --agent claude-code")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	st, err := state.ReadProject(cwd)
	require.NoError(t, err)
	assert.Empty(t, st.Installed, "state entry should be gone after remove")
	assert.NoFileExists(t, filepath.Join(cwd, ".shopware-cli", "ai", "installed.json"), "empty state file should be removed")
}

func TestRemoveNotRecordedIsNoOp(t *testing.T) {
	rec := setup(t)

	res, err := runRemove(t, RemoveOptions{Name: "shopware-cli", Agent: "claude-code"})
	require.NoError(t, err)
	assert.False(t, res.Removed)
	assert.Equal(t, 0, rec.calls, "must not touch skills when nothing is recorded")
}

func TestRemoveDryRunTouchesNothing(t *testing.T) {
	rec := setup(t)

	_, err := runAdd(t, Options{Name: "shopware-cli", Agent: "claude-code"})
	require.NoError(t, err)
	require.Equal(t, 1, rec.calls)

	res, err := runRemove(t, RemoveOptions{Name: "shopware-cli", Agent: "claude-code", DryRun: true})
	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.Equal(t, 1, rec.calls, "dry-run must not call skills")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	st, err := state.ReadProject(cwd)
	require.NoError(t, err)
	assert.Len(t, st.Installed, 1, "dry-run must not change state")
}

func TestRemoveRecordedButUnknownIntegration(t *testing.T) {
	rec := setup(t)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, state.SaveProject(cwd, state.File{Installed: []state.InstalledEntry{
		{Name: "legacy-skill", Agent: "claude-code", Scope: state.ScopeProject},
	}}))

	res, err := runRemove(t, RemoveOptions{Name: "legacy-skill", Agent: "claude-code"})
	require.NoError(t, err)
	assert.True(t, res.Removed)
	assert.Equal(t, 1, rec.calls, "a recorded install should still be uninstalled via skills")
}

func TestRemoveGuards(t *testing.T) {
	rec := setup(t)

	_, err := runRemove(t, RemoveOptions{Name: "does-not-exist", Agent: "claude-code"})
	assert.ErrorContains(t, err, "unknown integration")

	_, err = runRemove(t, RemoveOptions{Name: "shopware-cli"})
	assert.ErrorContains(t, err, "--agent")

	assert.Equal(t, 0, rec.calls)
}

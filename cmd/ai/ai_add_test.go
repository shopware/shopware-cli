package ai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/state"
)

func TestSkillsAddArgs(t *testing.T) {
	src := "https://github.com/shopware/shopware-cli/tree/0.18.3/skills/shopware-cli"
	got := strings.Join(skillsAddArgs(src, "claude-code", true, true), " ")
	want := "npx --yes skills@" + skillsVersion + " add " + src + " --agent claude-code --global -y"
	assert.Equal(t, want, got)

	got = strings.Join(skillsAddArgs("a", "claude-code", false, false), " ")
	assert.NotContains(t, got, "--global")
	assert.NotContains(t, got, " -y")
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

	_, ok = findInstall(f, "a", "claude-code", state.ScopeProject)
	assert.False(t, ok, "different scope is a different install")
}

func TestWriteAddResultJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeAddResult(&buf, formatJSON, addResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal,
		ResolvedRevision: "0.18.3", Command: []string{"npx", "skills"},
	}))
	assert.JSONEq(t, `{"name":"shopware-cli","agent":"claude-code","scope":"global","requestedTag":"","resolvedRevision":"0.18.3","dryRun":false,"command":["npx","skills"]}`, buf.String())
}

func TestWriteAddResultTableDryRun(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeAddResult(&buf, formatTable, addResult{
		Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal,
		DryRun: true, Command: []string{"npx", "skills", "add"},
	}))
	assert.Contains(t, buf.String(), "[dry-run]")
	assert.Contains(t, buf.String(), "npx skills add")
}

// --- integration over the command ---

// skillsCall captures how runSkills was invoked by the command under test.
type skillsCall struct {
	calls    int
	lastArgv []string
}

// setupAdd points the install-state at a temp config dir and replaces runSkills
// with a fake for the duration of the test. All runAdd calls in one test share
// this config dir, so idempotency across calls can be observed.
func setupAdd(t *testing.T) *skillsCall {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("HOME", tmp)                                     // macOS UserConfigDir base (global state)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdgcfg")) // Linux UserConfigDir base (global state)
	t.Setenv("PROJECT_ROOT", tmp)                             // project scope resolves here
	t.Chdir(tmp)

	rec := &skillsCall{}
	prev := runSkills
	runSkills = func(_ context.Context, argv []string, _ string, _ io.Writer) error {
		rec.calls++
		rec.lastArgv = argv
		return nil
	}
	t.Cleanup(func() { runSkills = prev })

	// Stub tag resolution so git-delivery tests need no network.
	prevResolve := resolveLatestTag
	resolveLatestTag = func(_ context.Context, _ string) (string, error) { return "0.1.9", nil }
	t.Cleanup(func() { resolveLatestTag = prevResolve })

	// Stub the compatibility check to "compatible" by default; tests override it.
	prevCompat := runCompatCheck
	runCompatCheck = func(_ context.Context, _, _, _, _ string, _ io.Writer) error { return nil }
	t.Cleanup(func() { runCompatCheck = prevCompat })

	// Stub tag verification to "exists" so explicit-tag tests need no network.
	prevVerify := verifyTag
	verifyTag = func(_ context.Context, _, _ string) error { return nil }
	t.Cleanup(func() { verifyTag = prevVerify })

	return rec
}

// runAdd drives the add flow directly (not the cobra layer); progress output is
// discarded. All runAdd calls in one test share the config dir set by setupAdd.
func runAdd(t *testing.T, o addOptions) (addResult, error) {
	t.Helper()

	var progress bytes.Buffer

	return performAdd(t.Context(), o, &progress)
}

func TestAddDryRunTouchesNothing(t *testing.T) {
	rec := setupAdd(t)

	res, err := runAdd(t, addOptions{name: "shopware-cli", agent: "claude-code", global: true, dryRun: true})
	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.Equal(t, 0, rec.calls, "dry-run must not run skills")

	st, err := state.Read()
	require.NoError(t, err)
	assert.Empty(t, st.Installed, "dry-run must not write state")
}

func TestAddInstallsAndIsIdempotent(t *testing.T) {
	rec := setupAdd(t)

	res, err := runAdd(t, addOptions{name: "shopware-cli", agent: "claude-code", global: true})
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls)
	assert.Equal(t, actionInstalled, res.Action)
	// An empty CLI version makes a bundled install fall back to the resolved
	// latest tag (stubbed to 0.1.9), pinned via a tree URL.
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "github.com/shopware/shopware-cli/tree/0.1.9/skills/shopware-cli")

	st, err := state.Read()
	require.NoError(t, err)
	require.Len(t, st.Installed, 1)
	assert.Equal(t, state.ScopeGlobal, st.Installed[0].Scope)

	// Second identical add: skills.sh runs again (idempotent, owns the disk), the
	// record stays one entry, and the outcome is reported as unchanged.
	res, err = runAdd(t, addOptions{name: "shopware-cli", agent: "claude-code", global: true})
	require.NoError(t, err)
	assert.Equal(t, 2, rec.calls, "repeated add re-runs skills.sh")
	assert.Equal(t, actionUnchanged, res.Action)

	st, err = state.Read()
	require.NoError(t, err)
	assert.Len(t, st.Installed, 1, "repeated add must not grow the record")
}

func TestAddReportsAction(t *testing.T) {
	setupAdd(t)

	res, err := runAdd(t, addOptions{name: "shopware-cli@0.18.3", agent: "claude-code", global: true})
	require.NoError(t, err)
	assert.Equal(t, actionInstalled, res.Action)

	res, err = runAdd(t, addOptions{name: "shopware-cli@0.18.3", agent: "claude-code", global: true})
	require.NoError(t, err)
	assert.Equal(t, actionUnchanged, res.Action)

	res, err = runAdd(t, addOptions{name: "shopware-cli@0.18.4", agent: "claude-code", global: true})
	require.NoError(t, err)
	assert.Equal(t, actionUpdated, res.Action)
	assert.Equal(t, "0.18.3", res.PreviousRevision)
	assert.Equal(t, "0.18.4", res.ResolvedRevision)
}

func TestAddProjectScopeWritesToProjectRoot(t *testing.T) {
	rec := setupAdd(t)

	res, err := runAdd(t, addOptions{name: "shopware-cli", agent: "claude-code"})
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls)
	assert.Equal(t, state.ScopeProject, res.Scope)
	assert.NotContains(t, strings.Join(rec.lastArgv, " "), "--global")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	st, err := state.ReadProject(cwd)
	require.NoError(t, err)
	require.Len(t, st.Installed, 1)
	assert.Equal(t, state.ScopeProject, st.Installed[0].Scope)

	// The global state stays empty.
	global, err := state.Read()
	require.NoError(t, err)
	assert.Empty(t, global.Installed)
}

func TestAddGitDeliveryResolvesLatestTagAndInstalls(t *testing.T) {
	rec := setupAdd(t) // stubs resolveLatestTag → "0.1.9"

	// No @tag → resolve the latest release from the git repository.
	res, err := runAdd(t, addOptions{name: "deployment-helper", agent: "claude-code"})
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls)
	assert.Equal(t, "0.1.9", res.ResolvedRevision)
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "github.com/shopware/deployment-helper/tree/0.1.9/skills/deployment-helper")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	st, err := state.ReadProject(cwd)
	require.NoError(t, err)
	require.Len(t, st.Installed, 1)
	assert.Equal(t, "0.1.9", st.Installed[0].ResolvedRevision)
}

func TestAddGitCompatCheckFailureAbortsInstall(t *testing.T) {
	rec := setupAdd(t)
	runCompatCheck = func(_ context.Context, _, _, _, _ string, _ io.Writer) error {
		return errors.New("PHP 8.2+ required, found 8.1")
	}

	_, err := runAdd(t, addOptions{name: "deployment-helper", agent: "claude-code"})
	assert.ErrorContains(t, err, "PHP 8.2+")
	assert.Equal(t, 0, rec.calls, "install must not run when the compatibility check fails")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	st, err := state.ReadProject(cwd)
	require.NoError(t, err)
	assert.Empty(t, st.Installed, "no state is written when the compatibility check fails")
}

func TestAddGitDeliveryGlobalNotSupported(t *testing.T) {
	rec := setupAdd(t)

	_, err := runAdd(t, addOptions{name: "deployment-helper", agent: "claude-code", global: true})
	assert.ErrorContains(t, err, "must be installed into a project")
	assert.Equal(t, 0, rec.calls)
}

func TestAddGuards(t *testing.T) {
	rec := setupAdd(t)

	_, err := runAdd(t, addOptions{name: "does-not-exist", agent: "claude-code", global: true})
	assert.ErrorContains(t, err, "unknown integration")

	_, err = runAdd(t, addOptions{name: "shopware-cli", global: true})
	assert.ErrorContains(t, err, "--agent")

	_, err = runAdd(t, addOptions{name: "shopware-cli", agent: "*", global: true})
	assert.ErrorContains(t, err, "single agent")

	_, err = runAdd(t, addOptions{name: "shopware-cli", agent: "claude-code,cursor", global: true})
	assert.ErrorContains(t, err, "single agent")

	assert.Equal(t, 0, rec.calls)
}

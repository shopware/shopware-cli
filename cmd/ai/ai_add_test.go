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
	got := strings.Join(skillsAddArgs("shopware/shopware-cli@0.18.3", "shopware-cli", "claude-code", true, true), " ")
	want := "npx --yes skills@" + skillsVersion + " add shopware/shopware-cli@0.18.3 --skill shopware-cli --agent claude-code --global -y"
	assert.Equal(t, want, got)

	got = strings.Join(skillsAddArgs("a", "s", "claude-code", false, false), " ")
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

func TestIsInstalled(t *testing.T) {
	f := state.File{Installed: []state.InstalledEntry{
		{Name: "a", Agent: "claude-code", Scope: state.ScopeGlobal, ResolvedRevision: "1"},
	}}

	assert.True(t, isInstalled(f, addResult{Name: "a", Agent: "claude-code", Scope: state.ScopeGlobal, ResolvedRevision: "1"}))
	// different revision → not installed (an update)
	assert.False(t, isInstalled(f, addResult{Name: "a", Agent: "claude-code", Scope: state.ScopeGlobal, ResolvedRevision: "2"}))
	// different agent → not installed
	assert.False(t, isInstalled(f, addResult{Name: "a", Agent: "codex", Scope: state.ScopeGlobal, ResolvedRevision: "1"}))
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
	t.Chdir(tmp)                                              // project state lives in the current directory

	rec := &skillsCall{}
	prev := runSkills
	runSkills = func(_ context.Context, argv []string, _ io.Writer) error {
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

	return rec
}

// runAdd runs `ai add` in isolation: it resets flags, parses args, and invokes
// RunE directly. Calling Execute() on the shared command tree would delegate to
// the global root (os.Args), so we drive the subcommand itself.
func runAdd(t *testing.T, args ...string) (string, error) {
	t.Helper()

	// Reset the shared command's flags so values do not leak between runs.
	for name, def := range map[string]string{
		"agent": "", "global": "false", "dry-run": "false", "format": "table",
	} {
		_ = aiAddCmd.Flags().Set(name, def)
	}

	var buf bytes.Buffer
	aiAddCmd.SetOut(&buf)
	aiAddCmd.SetErr(&buf)
	aiAddCmd.SetContext(t.Context())

	if err := aiAddCmd.ParseFlags(args); err != nil {
		return buf.String(), err
	}

	err := aiAddCmd.RunE(aiAddCmd, aiAddCmd.Flags().Args())

	return buf.String(), err
}

func TestAddDryRunTouchesNothing(t *testing.T) {
	rec := setupAdd(t)

	out, err := runAdd(t, "shopware-cli", "--agent", "claude-code", "--global", "--dry-run")
	require.NoError(t, err)

	assert.Contains(t, out, "[dry-run]")
	assert.Equal(t, 0, rec.calls, "dry-run must not run skills")

	st, err := state.Read()
	require.NoError(t, err)
	assert.Empty(t, st.Installed, "dry-run must not write state")
}

func TestAddInstallsAndIsIdempotent(t *testing.T) {
	rec := setupAdd(t)

	_, err := runAdd(t, "shopware-cli", "--agent", "claude-code", "--global")
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls)
	joined := strings.Join(rec.lastArgv, " ")
	assert.Contains(t, joined, "add shopware/shopware-cli")
	assert.Contains(t, joined, "--skill shopware-cli")
	assert.Contains(t, joined, "--agent claude-code")
	assert.Contains(t, joined, "--global")

	st, err := state.Read()
	require.NoError(t, err)
	require.Len(t, st.Installed, 1)
	assert.Equal(t, "shopware-cli", st.Installed[0].Name)
	assert.Equal(t, state.ScopeGlobal, st.Installed[0].Scope)

	// Second identical add: skills not run again, state unchanged.
	_, err = runAdd(t, "shopware-cli", "--agent", "claude-code", "--global")
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls, "repeated add with same revision must be a no-op")
}

func TestAddProjectScopeWritesToCurrentDir(t *testing.T) {
	rec := setupAdd(t)

	// No --global → project scope, installed into the current directory.
	_, err := runAdd(t, "shopware-cli", "--agent", "claude-code")
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls)
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
	_, err := runAdd(t, "deployment-helper", "--agent", "claude-code")
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls)

	joined := strings.Join(rec.lastArgv, " ")
	assert.Contains(t, joined, "add shopware/deployment-helper@0.1.9")
	assert.Contains(t, joined, "--skill deployment-helper")

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

	_, err := runAdd(t, "deployment-helper", "--agent", "claude-code")
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

	_, err := runAdd(t, "deployment-helper", "--agent", "claude-code", "--global")
	assert.ErrorContains(t, err, "global install of a git")
	assert.Equal(t, 0, rec.calls)
}

func TestAddGuards(t *testing.T) {
	rec := setupAdd(t)

	_, err := runAdd(t, "does-not-exist", "--agent", "claude-code", "--global")
	assert.ErrorContains(t, err, "unknown integration")

	_, err = runAdd(t, "shopware-cli", "--global")
	assert.ErrorContains(t, err, "--agent")

	assert.Equal(t, 0, rec.calls)
}

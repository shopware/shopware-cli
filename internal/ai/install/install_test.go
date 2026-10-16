package install

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/mcp"
	"github.com/shopware/shopware-cli/internal/ai/skills"
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

	n, tag = splitNameTag("shopware-cli@v1.2.3")
	assert.Equal(t, "shopware-cli", n)
	assert.Equal(t, "v1.2.3", tag)
}

// call captures how runSkills was invoked and what listSkills should return.
type call struct {
	calls     int
	lastArgv  []string
	lastDir   string
	installed []skills.Installed // what listSkills returns
}

// setup replaces runSkills and listSkills with fakes, points project resolution at
// a temp dir, and isolates HOME so MCP file writes land in a temp config.
func setup(t *testing.T) *call {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("PROJECT_ROOT", tmp)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(tmp)

	rec := &call{}
	prevRun := runSkills
	runSkills = func(_ context.Context, argv []string, dir string, _ io.Writer) error {
		rec.calls++
		rec.lastArgv = argv
		rec.lastDir = dir
		return nil
	}
	prevList := listSkills
	listSkills = func(_ context.Context, _ string, _ bool) ([]skills.Installed, error) {
		return rec.installed, nil
	}
	t.Cleanup(func() {
		runSkills = prevRun
		listSkills = prevList
	})

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

func TestAddWithTagIsVerbatim(t *testing.T) {
	rec := setup(t)

	res, err := runAdd(t, Options{Name: "deployment-helper@v0.1.9", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.Equal(t, "v0.1.9", res.ResolvedRevision)
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "github.com/shopware/deployment-helper/tree/v0.1.9/skills/deployment-helper")
}

func TestAddNoTagInstallsDefaultBranch(t *testing.T) {
	rec := setup(t)

	res, err := runAdd(t, Options{Name: "shopware-cli", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.Equal(t, "", res.ResolvedRevision)

	joined := strings.Join(rec.lastArgv, " ")
	assert.Contains(t, joined, "add shopware/shopware-cli --skill shopware-cli")
	assert.NotContains(t, joined, "/tree/")
}

func TestAddDryRunDoesNotRun(t *testing.T) {
	rec := setup(t)

	res, err := runAdd(t, Options{Name: "shopware-cli", Agent: "claude-code", Global: true, DryRun: true})
	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.Equal(t, 0, rec.calls, "dry-run must not run skills")
	assert.NotEmpty(t, res.Command, "dry-run still reports the command")
}

func TestAddProjectScopeRunsInProjectRoot(t *testing.T) {
	rec := setup(t)

	res, err := runAdd(t, Options{Name: "shopware-cli", Agent: "claude-code"})
	require.NoError(t, err)
	assert.Equal(t, ScopeProject, res.Scope)
	assert.NotEmpty(t, res.Dir)
	assert.Equal(t, res.Dir, rec.lastDir, "skills.sh runs in the project root")
}

func TestAddRejectsEmptyTag(t *testing.T) {
	rec := setup(t)

	_, err := runAdd(t, Options{Name: "deployment-helper@", Agent: "claude-code", Global: true})
	assert.ErrorContains(t, err, "empty version")
	assert.Equal(t, 0, rec.calls)
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

func TestAddMCPWritesConfig(t *testing.T) {
	rec := setup(t)

	// No --global flag: an MCP install is always global.
	res, err := runAdd(t, Options{Name: "meteor", Agent: "claude-code"})
	require.NoError(t, err)
	assert.Equal(t, ScopeGlobal, res.Scope)
	assert.Equal(t, 0, rec.calls, "mcp install must not run skills.sh")

	servers, err := mcp.List()
	require.NoError(t, err)
	require.Len(t, servers, 1)
	assert.Equal(t, "meteor", servers[0].Name)
	assert.Equal(t, "https://meteor.shopware.com/mcp", servers[0].Source)
}

func TestAddMCPRejectsTag(t *testing.T) {
	setup(t)

	_, err := runAdd(t, Options{Name: "meteor@1.0", Agent: "claude-code"})
	assert.ErrorContains(t, err, "cannot be pinned")
}

func TestAddMCPRejectsUnsupportedAgent(t *testing.T) {
	setup(t)

	_, err := runAdd(t, Options{Name: "meteor", Agent: "codex"})
	assert.ErrorContains(t, err, "not supported for MCP")
}

func TestRemoveMCPReportsWhetherPresent(t *testing.T) {
	setup(t)

	// Nothing installed yet → reports not removed.
	res, err := runRemove(t, RemoveOptions{Name: "meteor", Agent: "claude-code"})
	require.NoError(t, err)
	assert.False(t, res.Removed)
	assert.Equal(t, ScopeGlobal, res.Scope)

	// Install then remove → reports removed.
	_, err = runAdd(t, Options{Name: "meteor", Agent: "claude-code"})
	require.NoError(t, err)
	res, err = runRemove(t, RemoveOptions{Name: "meteor", Agent: "claude-code"})
	require.NoError(t, err)
	assert.True(t, res.Removed)
}

func TestRemoveMCPDryRunReportsPresenceWithoutWriting(t *testing.T) {
	setup(t)

	// Dry run with nothing installed must not claim a removal.
	res, err := runRemove(t, RemoveOptions{Name: "meteor", Agent: "claude-code", DryRun: true})
	require.NoError(t, err)
	assert.True(t, res.DryRun)
	assert.False(t, res.Removed, "dry-run must not report a removal when nothing is configured")

	servers, err := mcp.List()
	require.NoError(t, err)
	assert.Empty(t, servers, "dry-run must not write the config")

	// Dry run after an install reports it would remove, still without writing.
	_, err = runAdd(t, Options{Name: "meteor", Agent: "claude-code"})
	require.NoError(t, err)
	res, err = runRemove(t, RemoveOptions{Name: "meteor", Agent: "claude-code", DryRun: true})
	require.NoError(t, err)
	assert.True(t, res.Removed)

	servers, err = mcp.List()
	require.NoError(t, err)
	assert.Len(t, servers, 1, "dry-run leaves the installed server in place")
}

func TestRemoveInstalledRunsSkills(t *testing.T) {
	rec := setup(t)
	rec.installed = []skills.Installed{{Name: "shopware-cli", Agents: []string{"Claude Code"}}}

	res, err := runRemove(t, RemoveOptions{Name: "shopware-cli", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.True(t, res.Removed)
	assert.Equal(t, ScopeGlobal, res.Scope)
	assert.Equal(t, 1, rec.calls)
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "remove shopware-cli --agent claude-code --global")
}

func TestRemoveNothingInstalledIsNoOp(t *testing.T) {
	rec := setup(t) // nothing installed

	res, err := runRemove(t, RemoveOptions{Name: "shopware-cli", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.False(t, res.Removed)
	assert.Equal(t, 0, rec.calls, "must not run skills when nothing is installed")
}

func TestRemoveWrongAgentIsNoOp(t *testing.T) {
	rec := setup(t)
	// Installed for Claude Code only; removing for cursor must be a no-op.
	rec.installed = []skills.Installed{{Name: "shopware-cli", Agents: []string{"Claude Code"}}}

	res, err := runRemove(t, RemoveOptions{Name: "shopware-cli", Agent: "cursor", Global: true})
	require.NoError(t, err)
	assert.False(t, res.Removed)
	assert.Equal(t, 0, rec.calls)
}

func TestHasAgentMatchesDisplayNames(t *testing.T) {
	assert.True(t, hasAgent([]string{"Claude Code"}, "claude-code"))
	assert.True(t, hasAgent([]string{"GitHub Copilot"}, "github-copilot"))
	assert.False(t, hasAgent([]string{"Claude Code"}, "cursor"))
}

func TestRemoveGuards(t *testing.T) {
	rec := setup(t)

	_, err := runRemove(t, RemoveOptions{Name: "does-not-exist", Agent: "claude-code", Global: true})
	assert.ErrorContains(t, err, "unknown integration")

	_, err = runRemove(t, RemoveOptions{Name: "shopware-cli", Global: true})
	assert.ErrorContains(t, err, "--agent")

	assert.Equal(t, 0, rec.calls)
}

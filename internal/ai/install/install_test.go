package install

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// call captures how runSkills was invoked.
type call struct {
	calls    int
	lastArgv []string
	lastDir  string
}

// setup replaces runSkills with a fake and points project resolution at a temp
// dir so a project install needs no real Shopware project.
func setup(t *testing.T) *call {
	t.Helper()

	tmp := t.TempDir()
	t.Setenv("PROJECT_ROOT", tmp)
	t.Chdir(tmp)

	rec := &call{}
	prev := runSkills
	runSkills = func(_ context.Context, argv []string, dir string, _ io.Writer) error {
		rec.calls++
		rec.lastArgv = argv
		rec.lastDir = dir
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

func TestRemoveBuildsCommand(t *testing.T) {
	rec := setup(t)

	res, err := runRemove(t, RemoveOptions{Name: "shopware-cli", Agent: "claude-code", Global: true})
	require.NoError(t, err)
	assert.Equal(t, ScopeGlobal, res.Scope)
	assert.Equal(t, 1, rec.calls)
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "remove shopware-cli --agent claude-code --global")
}

func TestRemoveGuards(t *testing.T) {
	rec := setup(t)

	_, err := runRemove(t, RemoveOptions{Name: "does-not-exist", Agent: "claude-code", Global: true})
	assert.ErrorContains(t, err, "unknown integration")

	_, err = runRemove(t, RemoveOptions{Name: "shopware-cli", Global: true})
	assert.ErrorContains(t, err, "--agent")

	assert.Equal(t, 0, rec.calls)
}

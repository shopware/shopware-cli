package ai

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/state"
)

// runRemove drives the remove flow directly (not the cobra layer).
func runRemove(t *testing.T, o removeOptions) (removeResult, error) {
	t.Helper()

	var progress bytes.Buffer

	return performRemove(t.Context(), o, &progress)
}

func TestRemoveRecordedInstall(t *testing.T) {
	rec := setupAdd(t)

	_, err := runAdd(t, addOptions{name: "shopware-cli", agent: "claude-code"})
	require.NoError(t, err)
	require.Equal(t, 1, rec.calls)

	res, err := runRemove(t, removeOptions{name: "shopware-cli", agent: "claude-code"})
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
	rec := setupAdd(t)

	res, err := runRemove(t, removeOptions{name: "shopware-cli", agent: "claude-code"})
	require.NoError(t, err)
	assert.False(t, res.Removed)
	assert.Equal(t, 0, rec.calls, "must not touch skills when nothing is recorded")
}

func TestRemoveDryRunTouchesNothing(t *testing.T) {
	rec := setupAdd(t)

	_, err := runAdd(t, addOptions{name: "shopware-cli", agent: "claude-code"})
	require.NoError(t, err)
	require.Equal(t, 1, rec.calls)

	res, err := runRemove(t, removeOptions{name: "shopware-cli", agent: "claude-code", dryRun: true})
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
	rec := setupAdd(t)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	// A recorded install whose integration is no longer in the directory.
	require.NoError(t, state.SaveProject(cwd, state.File{Installed: []state.InstalledEntry{
		{Name: "legacy-skill", Agent: "claude-code", Scope: state.ScopeProject},
	}}))

	res, err := runRemove(t, removeOptions{name: "legacy-skill", agent: "claude-code"})
	require.NoError(t, err)
	assert.True(t, res.Removed)
	assert.Equal(t, 1, rec.calls, "a recorded install should still be uninstalled via skills")

	st, err := state.ReadProject(cwd)
	require.NoError(t, err)
	assert.Empty(t, st.Installed)
}

func TestRemoveGuards(t *testing.T) {
	rec := setupAdd(t)

	_, err := runRemove(t, removeOptions{name: "does-not-exist", agent: "claude-code"})
	assert.ErrorContains(t, err, "unknown integration")

	_, err = runRemove(t, removeOptions{name: "shopware-cli"})
	assert.ErrorContains(t, err, "--agent")

	assert.Equal(t, 0, rec.calls)
}

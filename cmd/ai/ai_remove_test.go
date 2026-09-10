package ai

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/state"
)

// runRemove runs `ai remove` in isolation, mirroring runAdd.
func runRemove(t *testing.T, args ...string) (string, error) {
	t.Helper()

	for name, def := range map[string]string{
		"agent": "", "global": "false", "dry-run": "false", "format": "table",
	} {
		_ = aiRemoveCmd.Flags().Set(name, def)
	}

	var buf bytes.Buffer
	aiRemoveCmd.SetOut(&buf)
	aiRemoveCmd.SetErr(&buf)
	aiRemoveCmd.SetContext(t.Context())

	if err := aiRemoveCmd.ParseFlags(args); err != nil {
		return buf.String(), err
	}

	err := aiRemoveCmd.RunE(aiRemoveCmd, aiRemoveCmd.Flags().Args())

	return buf.String(), err
}

func TestRemoveRecordedInstall(t *testing.T) {
	rec := setupAdd(t)

	_, err := runAdd(t, "shopware-cli", "--agent", "claude-code")
	require.NoError(t, err)
	require.Equal(t, 1, rec.calls)

	out, err := runRemove(t, "shopware-cli", "--agent", "claude-code")
	require.NoError(t, err)
	assert.Equal(t, 2, rec.calls) // add + remove
	assert.Contains(t, strings.Join(rec.lastArgv, " "), "remove shopware-cli --agent claude-code")
	assert.Contains(t, out, "Removed shopware-cli")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	st, err := state.ReadProject(cwd)
	require.NoError(t, err)
	assert.Empty(t, st.Installed, "state entry should be gone after remove")
}

func TestRemoveNotRecordedIsNoOp(t *testing.T) {
	rec := setupAdd(t)

	out, err := runRemove(t, "shopware-cli", "--agent", "claude-code")
	require.NoError(t, err)
	assert.Equal(t, 0, rec.calls, "must not touch skills when nothing is recorded")
	assert.Contains(t, out, "nothing to remove")
}

func TestRemoveDryRunTouchesNothing(t *testing.T) {
	rec := setupAdd(t)

	_, err := runAdd(t, "shopware-cli", "--agent", "claude-code")
	require.NoError(t, err)
	require.Equal(t, 1, rec.calls)

	out, err := runRemove(t, "shopware-cli", "--agent", "claude-code", "--dry-run")
	require.NoError(t, err)
	assert.Equal(t, 1, rec.calls, "dry-run must not call skills")
	assert.Contains(t, out, "[dry-run] would remove")

	cwd, err := os.Getwd()
	require.NoError(t, err)
	st, err := state.ReadProject(cwd)
	require.NoError(t, err)
	assert.Len(t, st.Installed, 1, "dry-run must not change state")
}

func TestRemoveGuards(t *testing.T) {
	rec := setupAdd(t)

	_, err := runRemove(t, "does-not-exist", "--agent", "claude-code")
	assert.ErrorContains(t, err, "unknown integration")

	_, err = runRemove(t, "shopware-cli")
	assert.ErrorContains(t, err, "--agent")

	assert.Equal(t, 0, rec.calls)
}

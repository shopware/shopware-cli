package ai

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
)

// executeAI runs a freshly constructed command with the given args and returns
// its combined output. A fresh command per call keeps cobra flag state isolated
// between tests.
func executeAI(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)

	err := cmd.Execute()

	return buf.String(), err
}

func TestListJSON(t *testing.T) {
	out, err := executeAI(t, newAIListCmd(), "--json")
	require.NoError(t, err)

	const want = `[
	  {"name":"shopware-cli","displayName":"Shopware CLI","type":"skill","provider":"shopware","description":"Use Shopware CLI effectively for project, extension, and account workflows.","status":"active","available":true},
	  {"name":"shopware-cli-docker","displayName":"Shopware CLI (Docker)","type":"skill","provider":"shopware","description":"Run commands in Docker-backed Shopware projects through Shopware CLI.","status":"active","available":true},
	  {"name":"deployment-helper","displayName":"Shopware Deployment Helper","type":"skill","provider":"shopware","description":"Use Shopware CLI and Deployment Helper together for build and deploy workflows.","status":"coming-soon","available":false}
	]`

	assert.JSONEq(t, want, out)
}

func TestListHumanContains(t *testing.T) {
	out, err := executeAI(t, newAIListCmd())
	require.NoError(t, err)

	for _, want := range []string{
		"shopware-cli",
		"shopware-cli-docker",
		"deployment-helper",
		"active",
		"coming-soon",
		"not yet released",
	} {
		assert.Contains(t, out, want)
	}
}

func TestListTypeSkillKeepsAll(t *testing.T) {
	out, err := executeAI(t, newAIListCmd(), "--type", "skill", "--json")
	require.NoError(t, err)

	assert.Contains(t, out, "shopware-cli")
	assert.Contains(t, out, "deployment-helper")
}

func TestListTypeMCPIsEmpty(t *testing.T) {
	out, err := executeAI(t, newAIListCmd(), "--type", "mcp", "--json")
	require.NoError(t, err)

	assert.JSONEq(t, `[]`, out)
}

func TestListTypeUnknownErrors(t *testing.T) {
	_, err := executeAI(t, newAIListCmd(), "--type", "foo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown --type "foo"`)
}

func TestListInstalledIsEmpty(t *testing.T) {
	// Redirect the user config dir so state.Path() points into an empty temp
	// dir regardless of the host: nothing writes install state until #1337.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	out, err := executeAI(t, newAIListCmd(), "--installed", "--json")
	require.NoError(t, err)

	assert.JSONEq(t, `[]`, out)
}

func TestAvailableLabel(t *testing.T) {
	assert.Equal(t, "yes", availableLabel(directory.Integration{Available: true}))
	assert.Equal(t, "no (not yet released)", availableLabel(directory.Integration{AvailabilityReason: "not yet released"}))
	assert.Equal(t, "no", availableLabel(directory.Integration{}))
}

// TestListInstalledShowsRecordedEntry writes an install-state file and asserts
// that --installed returns only the recorded integration.
func TestListInstalledShowsRecordedEntry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path, err := state.Path()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	const content = `{"version":1,"installed":[{"name":"shopware-cli","client":"codex","scope":"global","requestedTag":"latest","resolvedRevision":"abc"}]}`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	out, err := executeAI(t, newAIListCmd(), "--installed", "--json")
	require.NoError(t, err)

	var items []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "shopware-cli", items[0]["name"])
}

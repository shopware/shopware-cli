package ai

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

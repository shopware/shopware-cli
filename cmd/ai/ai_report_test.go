package ai

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runList(t *testing.T, args ...string) (string, error) {
	t.Helper()

	for name, def := range map[string]string{"type": "", "installed": "false", "format": "table"} {
		_ = aiListCmd.Flags().Set(name, def)
	}

	var buf bytes.Buffer
	aiListCmd.SetOut(&buf)
	aiListCmd.SetErr(&buf)
	aiListCmd.SetContext(t.Context())

	if err := aiListCmd.ParseFlags(args); err != nil {
		return buf.String(), err
	}

	err := aiListCmd.RunE(aiListCmd, aiListCmd.Flags().Args())

	return buf.String(), err
}

func runInfo(t *testing.T, args ...string) (string, error) {
	t.Helper()

	_ = aiInfoCmd.Flags().Set("format", "table")

	var buf bytes.Buffer
	aiInfoCmd.SetOut(&buf)
	aiInfoCmd.SetErr(&buf)
	aiInfoCmd.SetContext(t.Context())

	if err := aiInfoCmd.ParseFlags(args); err != nil {
		return buf.String(), err
	}

	err := aiInfoCmd.RunE(aiInfoCmd, aiInfoCmd.Flags().Args())

	return buf.String(), err
}

func TestListInstalledReportsRecordsPerAgentAndScope(t *testing.T) {
	setupAdd(t)

	// Same integration, two agents and scopes → two distinct rows.
	_, err := runAdd(t, "shopware-cli@0.18.3", "--agent", "claude-code", "--global")
	require.NoError(t, err)
	_, err = runAdd(t, "shopware-cli@0.18.4", "--agent", "codex") // project scope
	require.NoError(t, err)

	out, err := runList(t, "--installed")
	require.NoError(t, err)
	assert.Contains(t, out, "claude-code")
	assert.Contains(t, out, "codex")
	assert.Contains(t, out, "global")
	assert.Contains(t, out, "project")
	assert.Contains(t, out, "0.18.3")
	assert.Contains(t, out, "0.18.4")

	out, err = runList(t, "--installed", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"agent":"claude-code"`)
	assert.Contains(t, out, `"agent":"codex"`)
	assert.Contains(t, out, `"resolvedRevision":"0.18.4"`)
}

func TestInfoShowsInstalledRecords(t *testing.T) {
	setupAdd(t)

	// Nothing installed yet → empty installed list.
	out, err := runInfo(t, "shopware-cli", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"installed":[]`)

	_, err = runAdd(t, "shopware-cli@0.18.3", "--agent", "claude-code", "--global")
	require.NoError(t, err)

	out, err = runInfo(t, "shopware-cli", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"agent":"claude-code"`)
	assert.Contains(t, out, `"resolvedRevision":"0.18.3"`)

	out, err = runInfo(t, "shopware-cli")
	require.NoError(t, err)
	assert.Contains(t, out, "claude-code")
}

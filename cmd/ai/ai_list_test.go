package ai

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
)

// seedEnv isolates the install-state (global and project) in temp dirs.
func seedEnv(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "xdgcfg"))
	t.Setenv("PROJECT_ROOT", tmp)
	t.Chdir(tmp)
}

func TestWriteListJSON(t *testing.T) {
	entries := []directory.Integration{
		{
			Name: "x", DisplayName: "X", Type: directory.TypeSkill,
			Provider: "shopware", Description: "d", Status: directory.StatusActive,
			// fields below must not appear in the list shape
			Documentation: "https://example.test/x",
			Delivery:      directory.Delivery{Kind: directory.DeliveryBundled},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeList(&buf, formatJSON, entries))

	assert.JSONEq(t, `[{"name":"x","displayName":"X","type":"skill","provider":"shopware","description":"d","status":"active"}]`, buf.String())
}

func TestWriteListJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeList(&buf, formatJSON, nil))

	assert.JSONEq(t, `[]`, buf.String())
}

func TestWriteInstalledJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeInstalled(&buf, formatJSON, []state.InstalledEntry{
		{Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal, RequestedTag: "0.18.3", ResolvedRevision: "v0.18.3"},
	}))
	assert.JSONEq(t, `[{"name":"shopware-cli","agent":"claude-code","scope":"global","requestedTag":"0.18.3","resolvedRevision":"v0.18.3"}]`, buf.String())
}

func TestWriteInstalledJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeInstalled(&buf, formatJSON, nil))

	assert.JSONEq(t, `[]`, buf.String())
}

func TestInstalledRecordsMergesScopesAndAgents(t *testing.T) {
	seedEnv(t)

	require.NoError(t, state.Save(state.File{Installed: []state.InstalledEntry{
		{Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal, ResolvedRevision: "v0.18.3"},
	}}))
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, state.SaveProject(cwd, state.File{Installed: []state.InstalledEntry{
		{Name: "shopware-cli", Agent: "codex", Scope: state.ScopeProject, ResolvedRevision: "v0.18.4"},
	}}))

	records, err := installedRecords()
	require.NoError(t, err)
	require.Len(t, records, 2)

	byAgent := map[string]state.InstalledEntry{}
	for _, r := range records {
		byAgent[r.Agent] = r
	}
	assert.Equal(t, state.ScopeGlobal, byAgent["claude-code"].Scope)
	assert.Equal(t, state.ScopeProject, byAgent["codex"].Scope)
}

func TestRecordsInCatalog(t *testing.T) {
	records := []state.InstalledEntry{
		{Name: "shopware-cli", Agent: "claude-code"},
		{Name: "meteor", Agent: "claude-code"},
	}
	catalog := []directory.Integration{{Name: "meteor", Type: directory.TypeMCP}}

	got := recordsInCatalog(records, catalog)
	require.Len(t, got, 1)
	assert.Equal(t, "meteor", got[0].Name)
}

func TestInstalledRecordsForFiltersByName(t *testing.T) {
	seedEnv(t)

	require.NoError(t, state.Save(state.File{Installed: []state.InstalledEntry{
		{Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal},
	}}))

	got, err := installedRecordsFor("shopware-cli")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "claude-code", got[0].Agent)

	none, err := installedRecordsFor("deployment-helper")
	require.NoError(t, err)
	assert.Empty(t, none)
}

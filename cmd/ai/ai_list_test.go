package ai

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
)

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
	require.NoError(t, writeListJSON(&buf, entries))

	assert.JSONEq(t, `[{"name":"x","displayName":"X","type":"skill","provider":"shopware","description":"d","status":"active"}]`, buf.String())
}

func TestWriteListJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeListJSON(&buf, nil))

	assert.JSONEq(t, `[]`, buf.String())
}

func TestInstalledRecordsMergesScopesAndAgents(t *testing.T) {
	setupAdd(t)

	// Same integration, two agents and scopes → two distinct records.
	_, err := runAdd(t, addOptions{name: "shopware-cli@0.18.3", agent: "claude-code", global: true})
	require.NoError(t, err)
	_, err = runAdd(t, addOptions{name: "shopware-cli@0.18.4", agent: "codex"}) // project scope
	require.NoError(t, err)

	records, err := installedRecords()
	require.NoError(t, err)
	require.Len(t, records, 2)

	byAgent := map[string]state.InstalledEntry{}
	for _, r := range records {
		byAgent[r.Agent] = r
	}
	assert.Equal(t, state.ScopeGlobal, byAgent["claude-code"].Scope)
	assert.Equal(t, "0.18.3", byAgent["claude-code"].ResolvedRevision)
	assert.Equal(t, state.ScopeProject, byAgent["codex"].Scope)
	assert.Equal(t, "0.18.4", byAgent["codex"].ResolvedRevision)
}

func TestInstalledRecordsForFiltersByName(t *testing.T) {
	setupAdd(t)

	_, err := runAdd(t, addOptions{name: "shopware-cli", agent: "claude-code", global: true})
	require.NoError(t, err)

	got, err := installedRecordsFor("shopware-cli")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "claude-code", got[0].Agent)

	none, err := installedRecordsFor("deployment-helper")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestWriteInstalledJSON(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeInstalledJSON(&buf, []state.InstalledEntry{
		{Name: "shopware-cli", Agent: "claude-code", Scope: state.ScopeGlobal, RequestedTag: "0.18.3", ResolvedRevision: "0.18.3"},
	}))
	assert.JSONEq(t, `[{"name":"shopware-cli","agent":"claude-code","scope":"global","requestedTag":"0.18.3","resolvedRevision":"0.18.3"}]`, buf.String())
}

func TestWriteInstalledJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeInstalledJSON(&buf, nil))

	assert.JSONEq(t, `[]`, buf.String())
}

package ai

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/mcp"
	"github.com/shopware/shopware-cli/internal/ai/skills"
)

func TestWriteListJSON(t *testing.T) {
	entries := []directory.Integration{
		{
			Name: "x", DisplayName: "X", Type: directory.TypeSkill,
			Provider: "shopware", Description: "d", Status: directory.StatusActive,
			// fields below must not appear in the list shape
			Documentation: "https://example.test/x",
			Delivery:      directory.Delivery{Kind: directory.DeliveryGit, Repository: "shopware/x"},
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
	require.NoError(t, writeInstalled(&buf, formatJSON, []installedRow{
		{Name: "deployment-helper", Type: directory.TypeSkill, Scope: "project", Agents: []string{"Claude Code"}, Source: "shopware/deployment-helper"},
	}))
	assert.JSONEq(t, `[{"name":"deployment-helper","type":"skill","scope":"project","agents":["Claude Code"],"source":"shopware/deployment-helper"}]`, buf.String())
}

func TestWriteInstalledJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeInstalled(&buf, formatJSON, nil))

	assert.JSONEq(t, `[]`, buf.String())
}

func TestInstalledRecordsFiltersToCatalog(t *testing.T) {
	t.Setenv("PROJECT_ROOT", t.TempDir())

	prev := listSkills
	listSkills = func(_ context.Context, _ string, global bool) ([]skills.Installed, error) {
		if global {
			return []skills.Installed{
				{Name: "shopware-cli", Scope: "global", Agents: []string{"Claude Code"}},
				{Name: "some-other-skill", Scope: "global"}, // not in the directory
			}, nil
		}
		return []skills.Installed{
			{Name: "deployment-helper", Scope: "project", Agents: []string{"Codex"}},
		}, nil
	}
	t.Cleanup(func() { listSkills = prev })

	prevMCP := listMCP
	listMCP = func(string) ([]mcp.Installed, error) { return nil, nil }
	t.Cleanup(func() { listMCP = prevMCP })

	catalog, err := directory.Load().List(directory.ListOptions{})
	require.NoError(t, err)

	records, err := installedRecords(t.Context(), catalog)
	require.NoError(t, err)

	names := make([]string, len(records))
	for i, r := range records {
		names[i] = r.Name
	}
	assert.ElementsMatch(t, []string{"shopware-cli", "deployment-helper"}, names, "non-catalog skills are dropped")
}

func TestInstalledRecordsIncludesMCP(t *testing.T) {
	t.Setenv("PROJECT_ROOT", t.TempDir())

	prev := listSkills
	listSkills = func(_ context.Context, _ string, _ bool) ([]skills.Installed, error) { return nil, nil }
	t.Cleanup(func() { listSkills = prev })

	prevMCP := listMCP
	listMCP = func(string) ([]mcp.Installed, error) {
		return []mcp.Installed{
			{Name: "meteor", Source: "https://meteor.shopware.com/mcp"},
			{Name: "some-other-mcp", Source: "https://other/mcp"}, // not in the directory
		}, nil
	}
	t.Cleanup(func() { listMCP = prevMCP })

	catalog, err := directory.Load().List(directory.ListOptions{})
	require.NoError(t, err)

	records, err := installedRecords(t.Context(), catalog)
	require.NoError(t, err)

	require.Len(t, records, 1, "only catalog MCP servers are kept")
	assert.Equal(t, "meteor", records[0].Name)
	assert.Equal(t, directory.TypeMCP, records[0].Type)
	assert.Equal(t, "project", records[0].Scope)
	assert.Empty(t, records[0].Agents, "an MCP server in .mcp.json is not agent-specific")
}

package directory

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	d := Load()
	require.NotNil(t, d)

	assert.Equal(t, 1, d.Version)
	assert.NotEmpty(t, d.Integrations)

	_, ok := d.Get("shopware-cli")
	assert.True(t, ok)
}

func TestGet(t *testing.T) {
	d := Load()

	e, ok := d.Get("shopware-cli")
	require.True(t, ok)
	assert.Equal(t, "shopware-cli", e.Name)

	_, ok = d.Get("does-not-exist")
	assert.False(t, ok)
}

// TestIntegrationJSONNeverIncludesInternal asserts maintainer metadata never
// leaks into any user-facing output: the Integration struct that `ai info`
// marshals keeps the Internal field out of JSON via json:"-".
func TestIntegrationJSONNeverIncludesInternal(t *testing.T) {
	e := Integration{
		Name:     "example",
		Internal: &Internal{Maintainer: "@shopware/team"},
	}

	b, err := json.Marshal(e)
	require.NoError(t, err)

	out := strings.ToLower(string(b))
	assert.NotContains(t, out, "maintainer")
	assert.NotContains(t, out, "internal")
}

// fixtureDirectory is a fixed, minimal directory for logic tests, independent of
// the real hardwired data so adding integrations does not break these tests.
func fixtureDirectory() *Directory {
	return &Directory{
		Version: 1,
		Integrations: []Integration{
			{
				Name: "alpha-skill", DisplayName: "Alpha", Type: TypeSkill,
				Provider: "shopware", Description: "a", Status: StatusActive,
				Documentation: "https://example.test/a",
				Delivery:      Delivery{Kind: DeliveryBundled},
			},
			{
				Name: "beta-skill", DisplayName: "Beta", Type: TypeSkill,
				Provider: "shopware", Description: "b", Status: StatusActive,
				Documentation: "https://example.test/b",
				Delivery:      Delivery{Kind: DeliveryGit, Repository: "https://example.test/repo"},
			},
		},
	}
}

func TestListReturnsAll(t *testing.T) {
	got, err := fixtureDirectory().List(nil, ListOptions{})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestListTypeFilter(t *testing.T) {
	d := fixtureDirectory()

	skills, err := d.List(nil, ListOptions{Type: "skill"})
	require.NoError(t, err)
	assert.Len(t, skills, 2)

	// "mcp" is a reserved-but-known filter: empty result, no error.
	mcp, err := d.List(nil, ListOptions{Type: "mcp"})
	require.NoError(t, err)
	assert.Empty(t, mcp)

	_, err = d.List(nil, ListOptions{Type: "bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}

func TestListInstalledOnly(t *testing.T) {
	d := fixtureDirectory()

	got, err := d.List(map[string]bool{"beta-skill": true}, ListOptions{InstalledOnly: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "beta-skill", got[0].Name)

	none, err := d.List(nil, ListOptions{InstalledOnly: true})
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestInfo(t *testing.T) {
	d := fixtureDirectory()

	found, err := d.Info("alpha-skill")
	require.NoError(t, err)
	assert.Equal(t, "alpha-skill", found.Name)

	_, err = d.Info("does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown integration")
}

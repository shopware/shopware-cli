package directory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	d := Load()
	require.NotNil(t, d)

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

// fixtureDirectory is a fixed, minimal directory for logic tests, independent of
// the real hardwired data so adding integrations does not break these tests.
func fixtureDirectory() *Directory {
	return &Directory{
		Integrations: []Integration{
			{
				Name: "alpha-skill", DisplayName: "Alpha", Type: TypeSkill,
				Provider: "shopware", Description: "a", Status: StatusActive,
				Documentation: "https://example.test/a",
				Delivery:      Delivery{Kind: DeliveryGit, Repository: "shopware/alpha"},
			},
			{
				Name: "beta-skill", DisplayName: "Beta", Type: TypeSkill,
				Provider: "shopware", Description: "b", Status: StatusActive,
				Documentation: "https://example.test/b",
				Delivery:      Delivery{Kind: DeliveryGit, Repository: "shopware/beta"},
			},
		},
	}
}

func TestListReturnsAll(t *testing.T) {
	got, err := fixtureDirectory().List(ListOptions{})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}

func TestListTypeFilter(t *testing.T) {
	d := fixtureDirectory()

	skills, err := d.List(ListOptions{Type: "skill"})
	require.NoError(t, err)
	assert.Len(t, skills, 2)

	// "mcp" is a reserved-but-known filter: empty result, no error.
	mcp, err := d.List(ListOptions{Type: "mcp"})
	require.NoError(t, err)
	assert.Empty(t, mcp)

	_, err = d.List(ListOptions{Type: "bogus"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
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

func TestMeteorMCPEntry(t *testing.T) {
	e, ok := Load().Get("meteor")
	require.True(t, ok)

	assert.Equal(t, TypeMCP, e.Type)
	assert.Equal(t, DeliveryMCP, e.Delivery.Kind)
	assert.Equal(t, "https://meteor.shopware.com/mcp", e.Delivery.URL)
	assert.Equal(t, "http", e.Delivery.Transport)
}

func TestListMCPTypeFromRealDirectory(t *testing.T) {
	got, err := Load().List(ListOptions{Type: "mcp"})
	require.NoError(t, err)
	require.NotEmpty(t, got)

	for _, e := range got {
		assert.Equal(t, TypeMCP, e.Type)
	}
}

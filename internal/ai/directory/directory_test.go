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
	for _, entry := range d.Integrations {
		assert.NotEmpty(t, entry.Repository)
	}

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
	return &Directory{Integrations: []Integration{
		{Name: "alpha-skill", Description: "a", Repository: "https://example.test/repo"},
		{Name: "beta-skill", Description: "b", Repository: "https://example.test/repo"},
	}}
}

func TestListReturnsAll(t *testing.T) {
	got := fixtureDirectory().List(nil, false)
	assert.Len(t, got, 2)
}

func TestListInstalledOnly(t *testing.T) {
	d := fixtureDirectory()

	got := d.List(map[string]bool{"beta-skill": true}, true)
	require.Len(t, got, 1)
	assert.Equal(t, "beta-skill", got[0].Name)

	none := d.List(nil, true)
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

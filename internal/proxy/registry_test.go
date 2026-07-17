package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// withTempStateDir redirects StateDir to a temp directory for the duration
// of the test, so registry round-trips don't touch the real user config dir.
func withTempStateDir(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
}

func TestRegistryRoundTrip(t *testing.T) {
	withTempStateDir(t)

	reg, err := LoadRegistry()
	assert.NoError(t, err)
	assert.Empty(t, reg.Projects)

	reg.Upsert(ProjectEntry{ProjectRoot: "/shops/one", Hostname: "one.shopware.local"})
	assert.NoError(t, reg.Save())

	loaded, err := LoadRegistry()
	assert.NoError(t, err)
	assert.Len(t, loaded.Projects, 1)
	assert.Equal(t, "one.shopware.local", loaded.Projects[0].Hostname)
}

func TestRegistryUpsertReplacesByProjectRoot(t *testing.T) {
	withTempStateDir(t)

	var reg Registry
	reg.Upsert(ProjectEntry{ProjectRoot: "/shops/one", Hostname: "one.shopware.local"})
	reg.Upsert(ProjectEntry{ProjectRoot: "/shops/one", Hostname: "renamed.shopware.local"})

	assert.Len(t, reg.Projects, 1)
	assert.Equal(t, "renamed.shopware.local", reg.Projects[0].Hostname)
}

func TestRegistryRemove(t *testing.T) {
	withTempStateDir(t)

	var reg Registry
	reg.Upsert(ProjectEntry{ProjectRoot: "/shops/one", Hostname: "one.shopware.local"})

	assert.True(t, reg.Remove("/shops/one"))
	assert.Empty(t, reg.Projects)
	assert.False(t, reg.Remove("/shops/one"))
}

func TestRegistryFindByHostname(t *testing.T) {
	withTempStateDir(t)

	var reg Registry
	reg.Upsert(ProjectEntry{ProjectRoot: "/shops/one", Hostname: "shared.shopware.local"})

	_, found := reg.FindByHostname("shared.shopware.local", "/shops/one")
	assert.False(t, found, "should not collide with itself")

	entry, found := reg.FindByHostname("shared.shopware.local", "/shops/two")
	assert.True(t, found)
	assert.Equal(t, "/shops/one", entry.ProjectRoot)
}

func TestRegistryFind(t *testing.T) {
	withTempStateDir(t)

	reg := Registry{Projects: []ProjectEntry{{ProjectRoot: "/shops/one", Hostname: "one.shopware.local"}}}

	entry, found := reg.Find("/shops/one")
	assert.True(t, found)
	assert.Equal(t, "one.shopware.local", entry.Hostname)

	_, found = reg.Find("/shops/other")
	assert.False(t, found)
}

func TestRegistryRoundTripKeepsPreviousConfig(t *testing.T) {
	withTempStateDir(t)

	reg := Registry{}
	reg.Upsert(ProjectEntry{
		ProjectRoot:    "/shops/one",
		Hostname:       "one.shopware.local",
		PreviousAppURL: "http://127.0.0.1:8000",
		PreviousConfig: &ConfigURLState{RootURL: "http://127.0.0.1:8000", HasRoot: true},
	})
	assert.NoError(t, reg.Save())

	loaded, err := LoadRegistry()
	assert.NoError(t, err)
	entry, found := loaded.Find("/shops/one")
	assert.True(t, found)
	assert.NotNil(t, entry.PreviousConfig)
	assert.Equal(t, "http://127.0.0.1:8000", entry.PreviousConfig.RootURL)
	assert.True(t, entry.PreviousConfig.HasRoot)
	assert.False(t, entry.PreviousConfig.HasEnv)
}

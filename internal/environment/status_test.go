package environment

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCanRecordAfterLoadingNull(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "shopware-project.yml")
	path := OpenStore(configPath).path
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("null"), 0o600))

	env := sshEnv()
	require.NoError(t, OpenStore(configPath).Record(env, Result{Name: "production", Ready: true}))
	status, tested, _ := OpenStore(configPath).Lookup("production", env)
	assert.True(t, tested)
	assert.True(t, status.Ready)
}

func TestStoreRemembersResultsPerConfigFile(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "shopware-project.yml")
	env := sshEnv()
	testedAt := time.Date(2026, 11, 23, 9, 12, 0, 0, time.UTC)

	_, tested, changed := OpenStore(configPath).Lookup("production", env)
	assert.False(t, tested)
	assert.False(t, changed)

	require.NoError(t, OpenStore(configPath).Record(env, Result{Name: "production", Ready: true, Limited: true, TestedAt: testedAt}))

	status, tested, _ := OpenStore(configPath).Lookup("production", env)
	assert.True(t, tested)
	assert.True(t, status.Ready)
	assert.True(t, status.Limited)
	assert.True(t, testedAt.Equal(status.TestedAt))

	_, tested, _ = OpenStore(filepath.Join(t.TempDir(), "shopware-project.yml")).Lookup("production", env)
	assert.False(t, tested, "another project must not see the result")
}

func TestStoreTreatsEditedEnvironmentAsUntested(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	store := OpenStore(filepath.Join(t.TempDir(), "shopware-project.yml"))
	env := sshEnv()
	now := time.Now()
	require.NoError(t, store.Record(env, Result{Name: "production", Ready: true, TestedAt: now}))

	assert.True(t, store.Describe("production", env, now).Ready)

	env.SSH.Host = "new.example.com"
	overview := store.Describe("production", env, now)
	assert.False(t, overview.Ready)
	assert.Equal(t, "changed, not tested yet", overview.LastTest)
}

func TestStoreDescribesFailureWithItsReason(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	store := OpenStore(filepath.Join(t.TempDir(), "shopware-project.yml"))
	env := sshEnv()
	now := time.Now()

	assert.Equal(t, "not tested yet", store.Describe("production", env, now).LastTest)

	require.NoError(t, store.Record(env, Result{Name: "production", Reason: "directory missing", TestedAt: now}))
	overview := store.Describe("production", env, now)
	assert.False(t, overview.Ready)
	assert.Equal(t, "directory missing", overview.LastTest)
	assert.Equal(t, "ssh", overview.Type)
}

func TestStoreFollowsRenameAndRemoval(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	configPath := filepath.Join(t.TempDir(), "shopware-project.yml")
	store := OpenStore(configPath)
	env := sshEnv()
	require.NoError(t, store.Record(env, Result{Name: "staging", Ready: true, TestedAt: time.Now()}))

	require.NoError(t, store.Rename("staging", "preprod"))
	reopened := OpenStore(configPath)
	_, tested, _ := reopened.Lookup("staging", env)
	assert.False(t, tested)
	_, tested, _ = reopened.Lookup("preprod", env)
	assert.True(t, tested)

	require.NoError(t, reopened.Forget("preprod"))
	_, tested, _ = OpenStore(configPath).Lookup("preprod", env)
	assert.False(t, tested)
}

func TestFormatAge(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	for age, want := range map[time.Duration]string{
		10 * time.Second: "just now",
		5 * time.Minute:  "5 min ago",
		2 * time.Hour:    "2 h ago",
		30 * time.Hour:   "yesterday",
		72 * time.Hour:   "3 days ago",
	} {
		assert.Equal(t, want, FormatAge(now.Add(-age), now))
	}
}

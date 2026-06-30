package symfony

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/otiai10/copy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fixtureProject = "testdata/config-packages/project"

func TestNewProjectConfigMissingDirectory(t *testing.T) {
	pc, err := NewProjectConfig(t.TempDir())
	require.NoError(t, err)

	assert.Empty(t, pc.Environments())
	assert.Empty(t, pc.Files())

	cfg, err := pc.Config("dev")
	require.NoError(t, err)
	assert.Empty(t, cfg)
}

func TestEnvironments(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	// dev + prod come from directories, test from a when@test block.
	assert.ElementsMatch(t, []string{"dev", "prod", "test"}, pc.Environments())
}

func TestConfigMergesBaseFiles(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	cfg, err := pc.Config("dev")
	require.NoError(t, err)

	framework, ok := cfg["framework"].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "%env(APP_SECRET)%", framework["secret"])

	// cache.yaml (base) is merged into the same framework key, not overwritten.
	cache, ok := framework["cache"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "cache.adapter.filesystem", cache["app"])

	// dev/framework.yaml adds the profiler section for the dev environment.
	profiler, ok := framework["profiler"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, profiler["only_exceptions"])
}

func TestConfigEnvironmentOverridesBase(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	dev, err := pc.Config("dev")
	require.NoError(t, err)
	prod, err := pc.Config("prod")
	require.NoError(t, err)

	devCache := dev["framework"].(map[string]any)["cache"].(map[string]any)
	prodCache := prod["framework"].(map[string]any)["cache"].(map[string]any)

	// dev keeps the base filesystem adapter, prod overrides it via prod/cache.yaml.
	assert.Equal(t, "cache.adapter.filesystem", devCache["app"])
	assert.Equal(t, "cache.adapter.redis", prodCache["app"])

	// The override only replaces app; the base pools map survives the merge.
	pools, ok := prodCache["pools"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, pools, "my_pool")

	// prod has no profiler (that section is dev-only).
	assert.NotContains(t, prod["framework"].(map[string]any), "profiler")
}

func TestConfigWhenBlockOnlyAppliesToItsEnvironment(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	testCfg, err := pc.Config("test")
	require.NoError(t, err)
	devCfg, err := pc.Config("dev")
	require.NoError(t, err)

	testFramework := testCfg["framework"].(map[string]any)
	assert.Equal(t, true, testFramework["test"])

	// when@test only contributes to the test environment.
	assert.NotContains(t, devCfg["framework"].(map[string]any), "test")
}

func TestGetConfigValue(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	value, ok, err := pc.GetConfigValue("prod", "framework.cache.app")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "cache.adapter.redis", value)

	_, ok, err = pc.GetConfigValue("prod", "framework.does.not.exist")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestPackageConfig(t *testing.T) {
	pc, err := NewProjectConfig(fixtureProject)
	require.NoError(t, err)

	value, ok, err := pc.PackageConfig("dev", "framework")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.IsType(t, map[string]any{}, value)

	_, ok, err = pc.PackageConfig("dev", "doctrine")
	require.NoError(t, err)
	assert.False(t, ok)
}

// copyFixture copies the static fixture project into a temp dir so write tests
// can mutate it without touching testdata.
func copyFixture(t *testing.T) string {
	t.Helper()

	dst := t.TempDir()
	require.NoError(t, copy.Copy(fixtureProject, dst))

	return dst
}

func TestSetConfigValueEditsExistingFileInPlace(t *testing.T) {
	root := copyFixture(t)

	pc, err := NewProjectConfig(root)
	require.NoError(t, err)

	require.NoError(t, pc.SetConfigValue("prod", "framework.cache.app", "cache.adapter.apcu"))

	// The value that already lived in prod/cache.yaml is updated there.
	content, err := os.ReadFile(filepath.Join(root, "config", "packages", "prod", "cache.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "cache.adapter.apcu")
	// The sibling key in the same file is preserved.
	assert.Contains(t, string(content), "default_redis_provider")

	// Reload-after-write means the new value is observable immediately.
	value, ok, err := pc.GetConfigValue("prod", "framework.cache.app")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "cache.adapter.apcu", value)
}

func TestSetConfigValuePreservesComments(t *testing.T) {
	root := copyFixture(t)

	pc, err := NewProjectConfig(root)
	require.NoError(t, err)

	require.NoError(t, pc.SetConfigValue("dev", "framework.secret", "changed"))

	content, err := os.ReadFile(filepath.Join(root, "config", "packages", "framework.yaml"))
	require.NoError(t, err)

	// The leading comment of the base file survives the rewrite.
	assert.Contains(t, string(content), "# framework configuration shared by all environments")
	assert.Contains(t, string(content), "changed")
}

func TestSetConfigValueCreatesEnvironmentFile(t *testing.T) {
	root := copyFixture(t)

	pc, err := NewProjectConfig(root)
	require.NoError(t, err)

	// messenger is not configured anywhere; prod/ exists, so the value lands in
	// config/packages/prod/messenger.yaml.
	require.NoError(t, pc.SetConfigValue("prod", "messenger.transports.async", "%env(MESSENGER_TRANSPORT_DSN)%"))

	target := filepath.Join(root, "config", "packages", "prod", "messenger.yaml")
	assert.FileExists(t, target)

	value, ok, err := pc.GetConfigValue("prod", "messenger.transports.async")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "%env(MESSENGER_TRANSPORT_DSN)%", value)

	// dev never sees the prod-only file.
	_, ok, err = pc.GetConfigValue("dev", "messenger.transports.async")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestSetConfigValueCreatesBaseFile(t *testing.T) {
	root := copyFixture(t)

	pc, err := NewProjectConfig(root)
	require.NoError(t, err)

	require.NoError(t, pc.SetConfigValue(BaseEnvironment, "monolog.channels", []any{"deprecation"}))

	target := filepath.Join(root, "config", "packages", "monolog.yaml")
	assert.FileExists(t, target)

	value, ok, err := pc.GetConfigValue("dev", "monolog.channels")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, []any{"deprecation"}, value)
}

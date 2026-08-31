package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeUpgradeCheckPlugin(t *testing.T, root, name, composerJSON string) {
	t.Helper()
	dir := filepath.Join(root, "custom", "plugins", name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644))
}

func TestGetLocalExtensionsReadsComposerLockAndCustomPlugins(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PROJECT_ROOT", root)
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"),
		[]byte(`{"packages":[{"name":"shopware/core","version":"v6.6.5.0"}]}`), 0o644))

	writeUpgradeCheckPlugin(t, root, "FroshTest", `{
		"name": "frosh/frosh-test",
		"type": "shopware-platform-plugin",
		"version": "1.2.0",
		"require": { "shopware/core": "~6.6.0" },
		"extra": { "shopware-plugin-class": "FroshTest\\FroshTest" }
	}`)
	// A plugin without a parseable version falls back to 1.0.0.
	writeUpgradeCheckPlugin(t, root, "NoVersion", `{
		"name": "frosh/no-version",
		"type": "shopware-platform-plugin",
		"require": { "shopware/core": "~6.6.0" },
		"extra": { "shopware-plugin-class": "NoVersion\\NoVersion" }
	}`)

	coreVersion, extensions, err := getLocalExtensions()
	require.NoError(t, err)
	// The v prefix must be trimmed or the store compatibility API gets fed
	// a version it does not know.
	assert.Equal(t, "6.6.5.0", coreVersion.String())
	assert.Equal(t, "1.2.0", extensions["FroshTest"])
	assert.Equal(t, "1.0.0", extensions["NoVersion"])
}

func TestGetLocalExtensionsErrorsWhenCoreMissing(t *testing.T) {
	t.Run("missing composer.lock", func(t *testing.T) {
		t.Setenv("PROJECT_ROOT", t.TempDir())
		_, _, err := getLocalExtensions()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read composer.lock")
	})

	t.Run("core not in the lock file", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv("PROJECT_ROOT", root)
		require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"),
			[]byte(`{"packages":[{"name":"vendor/other","version":"1.0.0"}]}`), 0o644))

		_, _, err := getLocalExtensions()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shopware/core package not found in composer.lock")
	})
}

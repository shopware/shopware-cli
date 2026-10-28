package verifier

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
)

func TestValidationComposerJSONIgnoresPackageIdentity(t *testing.T) {
	first, err := validationComposerJSON([]byte(`{
		"name": "acme/first",
		"type": "shopware-platform-plugin",
		"autoload": {"psr-4": {"Acme\\First\\": "src/"}},
		"extra": {"shopware-plugin-class": "Acme\\First\\First"},
		"require": {"shopware/core": "~6.6.0"}
	}`))
	require.NoError(t, err)

	second, err := validationComposerJSON([]byte(`{
		"name": "acme/second",
		"version": "2.0.0",
		"autoload": {"psr-4": {"Acme\\Second\\": "src/"}},
		"require": {"shopware/core": "~6.6.0"}
	}`))
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))
}

func TestValidationComposerJSONRequiresSuggestedPackages(t *testing.T) {
	result, err := validationComposerJSON([]byte(`{
		"require": {"shopware/core": "~6.6.0", "psr/log": "^3.0"},
		"suggest": {"psr/log": "already required", "psr/container": "optional"}
	}`))
	require.NoError(t, err)

	var data struct {
		Require map[string]string `json:"require"`
	}
	require.NoError(t, json.Unmarshal(result, &data))

	assert.Equal(t, map[string]string{
		"shopware/core": "~6.6.0",
		"psr/log":       "^3.0",
		"psr/container": "*",
	}, data.Require)
}

func TestValidationComposerJSONKeepsResolutionSettings(t *testing.T) {
	result, err := validationComposerJSON([]byte(`{
		"require": {"shopware/core": "*"},
		"repositories": [{"type": "composer", "url": "https://packages.example.com"}],
		"minimum-stability": "dev",
		"prefer-stable": true
	}`))
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(result, &data))

	assert.ElementsMatch(t, []string{"require", "repositories", "minimum-stability", "prefer-stable"}, slices.Collect(maps.Keys(data)))
}

func TestPrepareComposerAutoloadKeepsExistingVendor(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), []byte(`{"require": {"shopware/core": "*"}}`), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "vendor"), 0o755))

	autoloadFile, err := prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir, CheckAgainst: "lowest"})
	require.NoError(t, err)

	assert.Equal(t, filepath.Join(rootDir, "vendor", "autoload.php"), autoloadFile)

	entries, err := os.ReadDir(rootDir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"composer.json", "vendor"}, entryNames(entries))
}

func TestPrepareComposerAutoloadRequiresProjectVendor(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), []byte(`{"require": {"shopware/core": "*"}}`), 0o644))

	_, err := prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir, IsProject: true})
	require.Error(t, err)

	assert.NoDirExists(t, filepath.Join(rootDir, "vendor"))
	assert.NoDirExists(t, filepath.Join(rootDir, "var"))
}

func TestPrepareComposerAutoloadCache(t *testing.T) {
	composerJSON := []byte(`{"name": "acme/plugin", "require": {"shopware/core": "~6.6.0"}}`)
	dependencies, err := validationComposerJSON(composerJSON)
	require.NoError(t, err)

	cases := []struct {
		name         string
		cachedJSON   []byte
		age          time.Duration
		checkAgainst string
		reused       bool
	}{
		{name: "same dependencies", cachedJSON: dependencies, checkAgainst: "highest", reused: true},
		{name: "changed dependencies", cachedJSON: []byte(`{"require":{"shopware/core":"~6.5.0"}}`), checkAgainst: "highest"},
		{name: "expired", cachedJSON: dependencies, age: validationVendorTTL + time.Hour, checkAgainst: "highest"},
		{name: "other check-against", cachedJSON: dependencies, checkAgainst: "lowest"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Without composer on PATH, anything but a cache hit fails.
			t.Setenv("PATH", t.TempDir())

			rootDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), composerJSON, 0o644))

			workDir := filepath.Join(rootDir, extension.ValidationCacheDir, "composer-highest")
			cachedAutoload := filepath.Join(workDir, "vendor", "autoload.php")
			require.NoError(t, os.MkdirAll(filepath.Dir(cachedAutoload), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(workDir, "composer.json"), tc.cachedJSON, 0o644))
			require.NoError(t, os.WriteFile(cachedAutoload, []byte("<?php"), 0o644))
			modified := time.Now().Add(-tc.age)
			require.NoError(t, os.Chtimes(cachedAutoload, modified, modified))

			autoloadFile, err := prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir, CheckAgainst: tc.checkAgainst})
			if !tc.reused {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, cachedAutoload, autoloadFile)
		})
	}
}

func TestEnsureValidationCacheDirIgnoresItselfInGit(t *testing.T) {
	rootDir := t.TempDir()

	cacheDir, err := ensureValidationCacheDir(rootDir)
	require.NoError(t, err)

	gitignore, err := os.ReadFile(filepath.Join(cacheDir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(gitignore))
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

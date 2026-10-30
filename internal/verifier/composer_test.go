package verifier

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
)

func TestValidationComposerJSONDropsAutoload(t *testing.T) {
	result, err := validationComposerJSON([]byte(`{
		"name": "acme/plugin",
		"autoload": {"psr-4": {"Acme\\": "src/"}, "files": ["src/functions.php"]},
		"autoload-dev": {"classmap": ["tests/"]},
		"require": {"shopware/core": "~6.6.0"},
		"config": {"platform": {"php": "8.2"}}
	}`))
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(result, &data))

	assert.ElementsMatch(t, []string{"name", "require", "config"}, slices.Collect(maps.Keys(data)))
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

func TestPrepareComposerAutoloadKeepsExistingVendor(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), []byte(`{"require": {"shopware/core": "*"}}`), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(rootDir, "vendor"), 0o755))

	autoloadFile, toolErr := prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir, CheckAgainst: "lowest"}, testExecution())
	require.Nil(t, toolErr)

	assert.Equal(t, filepath.Join(rootDir, "vendor", "autoload.php"), autoloadFile)

	entries, err := os.ReadDir(rootDir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"composer.json", "vendor"}, entryNames(entries))
}

func TestPrepareComposerAutoloadRequiresProjectVendor(t *testing.T) {
	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), []byte(`{"require": {"shopware/core": "*"}}`), 0o644))

	_, err := prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir, IsProject: true}, testExecution())
	require.NotNil(t, err)

	assert.NoDirExists(t, filepath.Join(rootDir, "vendor"))
	assert.NoDirExists(t, filepath.Join(rootDir, "var"))
}

func TestPrepareComposerAutoloadUpdatesCacheOnEveryRun(t *testing.T) {
	// A fake composer records its arguments and creates the autoload file in
	// the directory it runs in.
	runsLog := filepath.Join(t.TempDir(), "runs.log")
	fakeTool(t, "composer", "echo \"$*\" >> '"+runsLog+"'\n/bin/mkdir -p vendor\n: > vendor/autoload.php\n")

	composerJSON := []byte(`{"name": "acme/plugin", "require": {"shopware/core": "~6.6.0"}}`)
	dependencies, err := validationComposerJSON(composerJSON)
	require.NoError(t, err)

	rootDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(rootDir, "composer.json"), composerJSON, 0o644))

	for _, checkAgainst := range []string{"highest", "highest", "lowest"} {
		autoloadFile, toolErr := prepareComposerAutoload(t.Context(), ToolConfig{RootDir: rootDir, CheckAgainst: checkAgainst}, testExecution())
		require.Nil(t, toolErr)

		workDir := filepath.Join(rootDir, extension.ValidationCacheDir, "composer-"+checkAgainst)
		assert.Equal(t, filepath.Join(workDir, "vendor", "autoload.php"), autoloadFile)
		assert.FileExists(t, autoloadFile)

		cached, err := os.ReadFile(filepath.Join(workDir, "composer.json"))
		require.NoError(t, err)
		assert.Equal(t, dependencies, cached)
	}

	runs, err := os.ReadFile(runsLog)
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(runs)), "\n")
	require.Len(t, lines, 3)
	assert.NotContains(t, lines[1], "--prefer-lowest")
	assert.Contains(t, lines[2], "--prefer-lowest")

	entries, err := os.ReadDir(rootDir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"composer.json", "var"}, entryNames(entries))
}

func TestEnsureValidationCacheDirIgnoresItselfInGit(t *testing.T) {
	rootDir := t.TempDir()

	cacheDir, err := ensureValidationCacheDir(rootDir)
	require.NoError(t, err)

	gitignore, err := os.ReadFile(filepath.Join(cacheDir, ".gitignore"))
	require.NoError(t, err)
	assert.Equal(t, "*\n", string(gitignore))
}

func testExecution() *ToolExecution {
	return NewToolExecution(func([]validation.CheckResult) bool { return true })
}

func entryNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

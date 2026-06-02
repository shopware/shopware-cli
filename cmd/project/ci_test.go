package project

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectCISafetyCheck(t *testing.T) {
	t.Run("allows dirty git working tree in CI", func(t *testing.T) {
		root := newDirtyGitRepository(t)

		err := projectCISafetyCheck(t.Context(), root, false, mapGetenv(map[string]string{"CI": "true"}))

		assert.NoError(t, err)
	})

	t.Run("allows dirty git working tree with force", func(t *testing.T) {
		root := newDirtyGitRepository(t)

		err := projectCISafetyCheck(t.Context(), root, true, mapGetenv(nil))

		assert.NoError(t, err)
	})

	t.Run("rejects dirty git working tree outside CI without force", func(t *testing.T) {
		root := newDirtyGitRepository(t)

		err := projectCISafetyCheck(t.Context(), root, false, mapGetenv(nil))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "project ci removes source files")
		assert.Contains(t, err.Error(), "--force")
	})

	t.Run("allows clean git working tree outside CI without force", func(t *testing.T) {
		root := newCleanGitRepository(t)

		err := projectCISafetyCheck(t.Context(), root, false, mapGetenv(nil))

		assert.NoError(t, err)
	})
}

func TestGenerateProjectSBOM(t *testing.T) {
	root := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{
		"name": "acme/shop",
		"version": "1.2.3"
	}`), 0o644))

	assert.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), []byte(`{
		"packages": [
			{
				"name": "symfony/console",
				"version": "v6.3.0",
				"type": "library",
				"license": ["MIT"],
				"require": {"php": ">=8.1"}
			}
		],
		"packages-dev": [
			{"name": "phpunit/phpunit", "version": "10.0.0", "license": ["BSD-3-Clause"]}
		]
	}`), 0o644))

	assert.NoError(t, generateProjectSBOM(t.Context(), root))

	data, err := os.ReadFile(filepath.Join(root, "sbom.cdx.json"))
	assert.NoError(t, err)

	doc := map[string]interface{}{}
	assert.NoError(t, json.Unmarshal(data, &doc))

	assert.Equal(t, "CycloneDX", doc["bomFormat"])
	assert.Equal(t, "1.7", doc["specVersion"])

	metadata := doc["metadata"].(map[string]interface{})
	component := metadata["component"].(map[string]interface{})
	assert.Equal(t, "acme/shop", component["name"])
	assert.Equal(t, "1.2.3", component["version"])

	components := doc["components"].([]interface{})
	assert.Len(t, components, 1, "dev dependencies excluded by default")
	assert.Equal(t, "console", components[0].(map[string]interface{})["name"])
}

func TestGenerateProjectSBOMSkipsWhenLockMissing(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, generateProjectSBOM(t.Context(), root))

	_, err := os.Stat(filepath.Join(root, "sbom.cdx.json"))
	assert.True(t, os.IsNotExist(err), "no SBOM should be written when composer.lock is absent")
}

func newDirtyGitRepository(t *testing.T) string {
	t.Helper()

	root := newCleanGitRepository(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "untracked.txt"), []byte("local work"), 0o644))

	return root
}

func newCleanGitRepository(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	runGit(t, root, "init")

	return root
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v failed: %s", args, output)
}

func mapGetenv(env map[string]string) func(string) string {
	return func(key string) string {
		return env[key]
	}
}

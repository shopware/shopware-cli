package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/git"
)

// clearCIEnv unsets every CI environment variable that ci.IsCI checks so that
// guardDestructiveCIRun is evaluated as if running on a local workspace, even
// when the test suite itself runs in CI. Keep this list in sync with
// internal/ci.ciEnvVars.
func clearCIEnv(t *testing.T) {
	t.Helper()
	for _, env := range []string{
		"CI",
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"BITBUCKET_BUILD_NUMBER",
		"JENKINS_URL",
		"TEAMCITY_VERSION",
		"BUILDKITE",
		"DRONE",
	} {
		t.Setenv(env, "")
	}
}

// initDirtyRepo creates a git repository with an untracked file, so its working
// tree is dirty.
func initDirtyRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	assert.NoError(t, git.Init(t.Context(), dir))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "main.ts"), []byte("x"), 0o644))
	return dir
}

func TestGuardDestructiveCIRun(t *testing.T) {
	t.Run("force always proceeds", func(t *testing.T) {
		clearCIEnv(t)
		assert.NoError(t, guardDestructiveCIRun(t.Context(), initDirtyRepo(t), true))
	})

	t.Run("CI environment proceeds", func(t *testing.T) {
		clearCIEnv(t)
		t.Setenv("CI", "true")
		assert.NoError(t, guardDestructiveCIRun(t.Context(), initDirtyRepo(t), false))
	})

	t.Run("dirty working tree refuses", func(t *testing.T) {
		clearCIEnv(t)
		err := guardDestructiveCIRun(t.Context(), initDirtyRepo(t), false)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "--force")
	})

	t.Run("clean working tree proceeds", func(t *testing.T) {
		clearCIEnv(t)
		dir := t.TempDir()
		assert.NoError(t, git.Init(t.Context(), dir))
		assert.NoError(t, guardDestructiveCIRun(t.Context(), dir, false))
	})

	t.Run("no git repository proceeds", func(t *testing.T) {
		clearCIEnv(t)
		assert.NoError(t, guardDestructiveCIRun(t.Context(), t.TempDir(), false))
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

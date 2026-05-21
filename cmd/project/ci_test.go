package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestSourceMapCleanup(t *testing.T) {
	t.Run("invalid directory", func(t *testing.T) {
		assert.NoError(t, cleanupJavaScriptSourceMaps("invalid-directory"))
	})

	t.Run("does not touch js", func(t *testing.T) {
		tmpDir := t.TempDir()

		assert.NoError(t, cleanupJavaScriptSourceMaps(tmpDir))

		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "random.js"), []byte("test"), 0o644))

		assert.NoError(t, cleanupJavaScriptSourceMaps(tmpDir))

		assert.FileExists(t, filepath.Join(tmpDir, "random.js"))
	})

	t.Run("removes map files", func(t *testing.T) {
		tmpDir := t.TempDir()

		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "foo.js.map"), []byte("test"), 0o644))

		assert.NoError(t, cleanupJavaScriptSourceMaps(tmpDir))

		assert.NoFileExists(t, filepath.Join(tmpDir, "foo.js.map"))
	})

	t.Run("remove sourcemap comments", func(t *testing.T) {
		tmpDir := t.TempDir()

		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.js"), []byte("console.log//# sourceMappingURL=test.js.map"), 0o644))
		assert.NoError(t, os.WriteFile(filepath.Join(tmpDir, "test.js.map"), []byte("test"), 0o644))

		assert.NoError(t, cleanupJavaScriptSourceMaps(tmpDir))

		content, err := os.ReadFile(filepath.Join(tmpDir, "test.js"))
		assert.NoError(t, err)

		assert.Equal(t, "console.log", string(content))
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

	cfg := &shop.ConfigBuildSBOM{Path: "build/sbom.cdx.json"}

	assert.NoError(t, generateProjectSBOM(t.Context(), root, cfg))

	data, err := os.ReadFile(filepath.Join(root, "build", "sbom.cdx.json"))
	assert.NoError(t, err)

	doc := map[string]interface{}{}
	assert.NoError(t, json.Unmarshal(data, &doc))

	assert.Equal(t, "CycloneDX", doc["bomFormat"])
	assert.Equal(t, "1.5", doc["specVersion"])

	metadata := doc["metadata"].(map[string]interface{})
	component := metadata["component"].(map[string]interface{})
	assert.Equal(t, "acme/shop", component["name"])
	assert.Equal(t, "1.2.3", component["version"])

	components := doc["components"].([]interface{})
	assert.Len(t, components, 1, "dev dependencies excluded by default")
	assert.Equal(t, "console", components[0].(map[string]interface{})["name"])
}

func TestGenerateProjectSBOMIncludeDev(t *testing.T) {
	root := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"name": "acme/shop"}`), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), []byte(`{
		"packages": [{"name": "symfony/console", "version": "v6.3.0"}],
		"packages-dev": [{"name": "phpunit/phpunit", "version": "10.0.0"}]
	}`), 0o644))

	cfg := &shop.ConfigBuildSBOM{IncludeDev: true}
	assert.NoError(t, generateProjectSBOM(t.Context(), root, cfg))

	data, err := os.ReadFile(filepath.Join(root, "sbom.cdx.json"))
	assert.NoError(t, err)

	doc := map[string]interface{}{}
	assert.NoError(t, json.Unmarshal(data, &doc))
	assert.Len(t, doc["components"].([]interface{}), 2)
}

func TestGenerateProjectSBOMSkipsWhenLockMissing(t *testing.T) {
	root := t.TempDir()
	assert.NoError(t, generateProjectSBOM(t.Context(), root, nil))

	_, err := os.Stat(filepath.Join(root, "sbom.cdx.json"))
	assert.True(t, os.IsNotExist(err), "no SBOM should be written when composer.lock is absent")
}

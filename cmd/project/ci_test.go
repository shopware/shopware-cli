package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

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

package projectbuild

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestGenerateProjectSBOMSkipsWhenLockMissing(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, generateProjectSBOM(t.Context(), root, "test"))
	assert.NoFileExists(t, filepath.Join(root, shop.DefaultProjectSBOMOutput))
}

func TestGenerateProjectSBOM(t *testing.T) {
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"),
		testhelper.ComposerJSON{Name: "acme/shop", Version: "1.2.3"}.String())
	testhelper.WriteFile(t, filepath.Join(root, "composer.lock"), `{
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
	}`)
	require.NoError(t, generateProjectSBOM(t.Context(), root, "build-test"))
	data, err := os.ReadFile(filepath.Join(root, shop.DefaultProjectSBOMOutput))
	require.NoError(t, err)
	doc := map[string]any{}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "CycloneDX", doc["bomFormat"])
	assert.Equal(t, "1.7", doc["specVersion"])
	assert.Contains(t, string(data), "build-test")
	assert.NotContains(t, string(data), "phpunit")
}

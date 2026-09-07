package shop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func writeMinimalComposerProject(t *testing.T, root string) {
	t.Helper()

	testhelper.WriteFile(t, filepath.Join(root, "composer.json"),
		testhelper.ComposerJSON{Name: "acme/shop", Version: "1.2.3"}.String())

	// The SBOM needs license and require metadata per package, which the
	// shared lock builder does not model.
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
}

func TestWriteProjectSBOM(t *testing.T) {
	root := t.TempDir()
	writeMinimalComposerProject(t, root)

	require.NoError(t, WriteProjectSBOM(t.Context(), root, ProjectSBOMOptions{
		ToolVersion: "test",
	}))

	data, err := os.ReadFile(filepath.Join(root, DefaultProjectSBOMOutput))
	require.NoError(t, err)

	doc := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(data, &doc))

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

func TestWriteProjectSBOMIncludeDevDependencies(t *testing.T) {
	root := t.TempDir()
	writeMinimalComposerProject(t, root)

	require.NoError(t, WriteProjectSBOM(t.Context(), root, ProjectSBOMOptions{
		IncludeDevDependencies: true,
		ToolVersion:            "test",
	}))

	data, err := os.ReadFile(filepath.Join(root, DefaultProjectSBOMOutput))
	require.NoError(t, err)

	doc := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(data, &doc))

	components := doc["components"].([]interface{})
	assert.Len(t, components, 2, "dev dependencies included when requested")
}

func TestWriteProjectSBOMCustomOutputPath(t *testing.T) {
	root := t.TempDir()
	writeMinimalComposerProject(t, root)

	outDir := t.TempDir()
	outFile := filepath.Join(outDir, "custom-sbom.json")

	require.NoError(t, WriteProjectSBOM(t.Context(), root, ProjectSBOMOptions{
		OutputPath:  outFile,
		ToolVersion: "test",
	}))

	data, err := os.ReadFile(outFile)
	require.NoError(t, err)

	doc := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "CycloneDX", doc["bomFormat"])

	_, err = os.Stat(filepath.Join(root, DefaultProjectSBOMOutput))
	assert.True(t, os.IsNotExist(err), "default path must not be written when --output is set")
}

func TestWriteProjectSBOMErrorsWhenLockMissing(t *testing.T) {
	root := t.TempDir()
	err := WriteProjectSBOM(t.Context(), root, ProjectSBOMOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "composer.lock not found")
}

func TestWriteProjectSBOMSkipsWhenLockMissingAndAllowed(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, WriteProjectSBOM(t.Context(), root, ProjectSBOMOptions{
		SkipMissingLock: true,
	}))

	_, err := os.Stat(filepath.Join(root, DefaultProjectSBOMOutput))
	assert.True(t, os.IsNotExist(err), "no SBOM should be written when composer.lock is absent")
}

func TestResolveProjectSBOMOutputPath(t *testing.T) {
	root := t.TempDir()

	defaultPath, err := ResolveProjectSBOMOutputPath(root, "")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, DefaultProjectSBOMOutput), defaultPath)

	abs := filepath.Join(root, "out.json")
	got, err := ResolveProjectSBOMOutputPath(root, abs)
	require.NoError(t, err)
	assert.Equal(t, abs, got)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	got, err = ResolveProjectSBOMOutputPath(root, "rel-sbom.json")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(cwd, "rel-sbom.json"), got)
}

func TestValidateProjectSBOMFormat(t *testing.T) {
	assert.NoError(t, ValidateProjectSBOMFormat(ProjectSBOMFormatCycloneDXJSON))
	assert.NoError(t, ValidateProjectSBOMFormat("CycloneDX-JSON"))

	err := ValidateProjectSBOMFormat("spdx-json")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SBOM format")
}

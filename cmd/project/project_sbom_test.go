package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

func TestGenerateProjectSBOMSkipsWhenLockMissing(t *testing.T) {
	// CI wrapper must keep the historical skip-on-missing-lock behaviour.
	root := t.TempDir()
	assert.NoError(t, generateProjectSBOM(t.Context(), root))

	_, err := os.Stat(filepath.Join(root, shop.DefaultProjectSBOMOutput))
	assert.True(t, os.IsNotExist(err), "no SBOM should be written when composer.lock is absent")
}

func TestGenerateProjectSBOM(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{
		"name": "acme/shop",
		"version": "1.2.3"
	}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), []byte(`{
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

	data, err := os.ReadFile(filepath.Join(root, shop.DefaultProjectSBOMOutput))
	assert.NoError(t, err)

	doc := map[string]interface{}{}
	assert.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "CycloneDX", doc["bomFormat"])
	assert.Equal(t, "1.7", doc["specVersion"])
}

func TestProjectSbomCommandUnsupportedFormat(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, projectSbomCmd.Flags().Set("format", "spdx-json"))
	require.NoError(t, projectSbomCmd.Flags().Set("output", ""))
	t.Cleanup(func() {
		_ = projectSbomCmd.Flags().Set("format", shop.ProjectSBOMFormatCycloneDXJSON)
		_ = projectSbomCmd.Flags().Set("output", "")
		_ = projectSbomCmd.Flags().Set("include-dev-dependencies", "false")
	})

	projectSbomCmd.SetContext(t.Context())
	err := projectSbomCmd.RunE(projectSbomCmd, []string{root})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported SBOM format")
}

func TestProjectSbomCommandSuccess(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"name":"acme/shop","version":"1.2.3"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), []byte(`{
		"packages":[{"name":"symfony/console","version":"v6.3.0","type":"library","license":["MIT"]}],
		"packages-dev":[]
	}`), 0o644))

	out := filepath.Join(root, "from-cmd.json")
	require.NoError(t, projectSbomCmd.Flags().Set("format", shop.ProjectSBOMFormatCycloneDXJSON))
	require.NoError(t, projectSbomCmd.Flags().Set("output", out))
	require.NoError(t, projectSbomCmd.Flags().Set("include-dev-dependencies", "false"))
	t.Cleanup(func() {
		_ = projectSbomCmd.Flags().Set("format", shop.ProjectSBOMFormatCycloneDXJSON)
		_ = projectSbomCmd.Flags().Set("output", "")
		_ = projectSbomCmd.Flags().Set("include-dev-dependencies", "false")
	})

	// Ensure the tool version used by the command path is stable in tests.
	prev := tui.AppVersion
	tui.AppVersion = "test"
	t.Cleanup(func() { tui.AppVersion = prev })

	projectSbomCmd.SetContext(t.Context())
	require.NoError(t, projectSbomCmd.RunE(projectSbomCmd, []string{root}))

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	doc := map[string]interface{}{}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "1.7", doc["specVersion"])
}

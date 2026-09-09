package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
	"github.com/shopware/shopware-cli/internal/tui"
)

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
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"),
		testhelper.ComposerJSON{Name: "acme/shop", Version: "1.2.3"}.String())
	testhelper.WriteFile(t, filepath.Join(root, "composer.lock"), `{
		"packages":[{"name":"symfony/console","version":"v6.3.0","type":"library","license":["MIT"]}],
		"packages-dev":[]
	}`)

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

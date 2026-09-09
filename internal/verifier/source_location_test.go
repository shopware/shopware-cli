package verifier

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/testhelper"
	"github.com/shopware/shopware-cli/internal/validation"
)

func TestZipValidationUsesArchiveRelativePaths(t *testing.T) {
	parent := t.TempDir()
	pluginDir := filepath.Join(parent, "SwagExample")
	testhelper.WriteFile(t, filepath.Join(pluginDir, "composer.json"), testhelper.PluginComposer("test/swag-example", "1.0.0", `SwagExample\SwagExample`).String())
	writeDeprecatedServicesXML(t, pluginDir)
	testhelper.WriteFile(t, filepath.Join(pluginDir, ".DS_Store"), "store")

	zipPath := filepath.Join(t.TempDir(), "SwagExample.zip")
	require.NoError(t, archiver.CreateZip(parent, zipPath))

	ext, err := extension.GetExtensionByZip(t.Context(), zipPath)
	require.NoError(t, err)
	require.NotEmpty(t, ext.GetPath())

	check := NewCheck()
	check.SetSourceRoot(ext.GetPath())
	require.NoError(t, SWCLI{}.Check(t.Context(), check, ToolConfig{
		Extension: ext,
		RootDir:   ext.GetPath(),
	}))

	results := check.GetResults()
	require.NotEmpty(t, results)

	extractRoot := ext.GetPath()
	for _, result := range results {
		assert.NotContains(t, result.Path, extractRoot)
		assert.NotContains(t, result.Message, extractRoot)
		assert.NotContains(t, result.Path, os.TempDir())
		assert.False(t, filepath.IsAbs(result.Path), "path %q should be relative", result.Path)
		if result.Path != "" {
			assert.Greater(t, result.Line, 0)
		}
	}

	var xmlFinding, zipFinding *validation.CheckResult
	for i := range results {
		switch results[i].Identifier {
		case "config.services_xml.deprecated":
			xmlFinding = &results[i]
		case "zip.disallowed_file":
			if strings.Contains(results[i].Path, ".DS_Store") {
				zipFinding = &results[i]
			}
		}
	}

	require.NotNil(t, xmlFinding)
	assert.Equal(t, "src/Resources/config/services.xml", xmlFinding.Path)
	require.NotNil(t, zipFinding)
	assert.Equal(t, ".DS_Store", zipFinding.Path)
}

func TestDirectoryValidationUsesExtensionRelativePaths(t *testing.T) {
	pluginDir := testhelper.NewPlugin(t, "SwagExample")
	writeDeprecatedServicesXML(t, pluginDir)

	ext, err := extension.GetExtensionByFolder(t.Context(), pluginDir)
	require.NoError(t, err)

	check := NewCheck()
	check.SetSourceRoot(pluginDir)
	require.NoError(t, SWCLI{}.Check(t.Context(), check, ToolConfig{
		Extension:         ext,
		RootDir:           pluginDir,
		InputWasDirectory: true,
	}))

	foundXML := false
	for _, result := range check.GetResults() {
		assert.NotContains(t, result.Path, pluginDir)
		assert.NotContains(t, result.Message, pluginDir)
		if result.Identifier == "config.services_xml.deprecated" {
			foundXML = true
			assert.Equal(t, "src/Resources/config/services.xml", result.Path)
			assert.Equal(t, 1, result.Line)
		}
	}
	assert.True(t, foundXML)
}

func writeDeprecatedServicesXML(t *testing.T, pluginDir string) {
	t.Helper()
	testhelper.WriteFile(t, filepath.Join(pluginDir, "src", "Resources", "config", "services.xml"), "<container/>")
}

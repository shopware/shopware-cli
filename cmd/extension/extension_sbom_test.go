package extension

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func writeSBOMCommandTestPlugin(t *testing.T) string {
	t.Helper()

	extDir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(extDir, "composer.json"), testhelper.ComposerJSON{
		Name:        "frosh/frosh-test",
		Type:        "shopware-platform-plugin",
		License:     "MIT",
		Version:     "1.0.0",
		Authors:     []string{"FriendsOfShopware"},
		Require:     map[string]string{"shopware/core": "~6.6.0"},
		PluginClass: `FroshTest\FroshTest`,
		Label:       map[string]string{"de-DE": "Test", "en-GB": "Test"},
		Psr4:        map[string]string{`FroshTest\`: "src/"},
	}.String())
	testhelper.WriteFile(t, filepath.Join(extDir, ".shopware-extension.yml"), `build:
  zip:
    composer:
      enabled: false
    assets:
      enabled: false
`)

	return extDir
}

func readSBOMFile(t *testing.T, path string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(content, &doc))

	return doc
}

func TestExtensionSbomCommandWritesDefaultOutput(t *testing.T) {
	extDir := writeSBOMCommandTestPlugin(t)

	extensionSbomCmd.SetContext(t.Context())
	require.NoError(t, extensionSbomCmd.RunE(extensionSbomCmd, []string{extDir}))

	doc := readSBOMFile(t, filepath.Join(extDir, "sbom.cdx.json"))
	component := doc["metadata"].(map[string]any)["component"].(map[string]any)
	assert.Equal(t, "pkg:composer/frosh/frosh-test@1.0.0", component["purl"])
	assert.Equal(t, "FriendsOfShopware", doc["metadata"].(map[string]any)["manufacturer"].(map[string]any)["name"])
}

func TestExtensionSbomCommandRejectsUnknownFormat(t *testing.T) {
	extDir := writeSBOMCommandTestPlugin(t)

	extensionSbomCmd.SetContext(t.Context())
	require.NoError(t, extensionSbomCmd.Flags().Set("format", "spdx-json"))
	t.Cleanup(func() { _ = extensionSbomCmd.Flags().Set("format", extensionSBOMFormatCycloneDXJSON) })

	err := extensionSbomCmd.RunE(extensionSbomCmd, []string{extDir})
	assert.Error(t, err)
}

func TestPackageWritesAndEmbedsSBOM(t *testing.T) {
	extDir := writeSBOMCommandTestPlugin(t)
	workDir := t.TempDir()
	t.Chdir(workDir)

	disableGit = true
	t.Cleanup(func() { disableGit = false })

	outputDir := filepath.Join(workDir, "dist")
	extensionPackageCmd.SetContext(t.Context())
	for flag, value := range map[string]string{
		"output-directory":  outputDir,
		"overwrite-version": "1.4.2",
		"sbom":              "true",
		"sbom-embed":        "true",
	} {
		require.NoError(t, extensionPackageCmd.Flags().Set(flag, value))
	}
	t.Cleanup(func() {
		_ = extensionPackageCmd.Flags().Set("output-directory", "")
		_ = extensionPackageCmd.Flags().Set("overwrite-version", "")
		_ = extensionPackageCmd.Flags().Set("sbom", "false")
		_ = extensionPackageCmd.Flags().Set("sbom-embed", "false")
	})

	require.NoError(t, extensionPackageCmd.RunE(extensionPackageCmd, []string{extDir}))

	// The sidecar describes the packaged files, including the overwritten version.
	doc := readSBOMFile(t, filepath.Join(outputDir, "FroshTest.cdx.json"))
	component := doc["metadata"].(map[string]any)["component"].(map[string]any)
	assert.Equal(t, "1.4.2", component["version"])

	reader, err := zip.OpenReader(filepath.Join(outputDir, "FroshTest.zip"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	require.Contains(t, names, "FroshTest/sbom.cdx.json")

	// Sidecar and embedded copy are the same document.
	embedded, err := reader.Open("FroshTest/sbom.cdx.json")
	require.NoError(t, err)
	t.Cleanup(func() { _ = embedded.Close() })

	var embeddedDoc map[string]any
	require.NoError(t, json.NewDecoder(embedded).Decode(&embeddedDoc))
	assert.Equal(t, doc["serialNumber"], embeddedDoc["serialNumber"])

	// The source folder stays untouched.
	assert.NoFileExists(t, filepath.Join(extDir, "sbom.cdx.json"))
}

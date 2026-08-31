package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestPackageDoesNotDeleteUnrelatedZipsInWorkingDirectory(t *testing.T) {
	extDir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(extDir, "composer.json"), testhelper.ComposerJSON{
		Name:        "frosh/frosh-test",
		Type:        "shopware-platform-plugin",
		License:     "MIT",
		Version:     "1.0.0",
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

	workDir := t.TempDir()
	decoyBackup := filepath.Join(workDir, "FroshTest-backup-2024.zip")
	decoyRelease := filepath.Join(workDir, "FroshTest-1.0.0.zip")
	require.NoError(t, os.WriteFile(decoyBackup, []byte("precious"), 0o644))
	require.NoError(t, os.WriteFile(decoyRelease, []byte("artifact"), 0o644))

	t.Chdir(workDir)

	disableGit = true
	t.Cleanup(func() { disableGit = false })

	outputDir := filepath.Join(workDir, "dist")
	extensionPackageCmd.SetContext(t.Context())
	require.NoError(t, extensionPackageCmd.Flags().Set("output-directory", outputDir))
	require.NoError(t, extensionPackageCmd.Flags().Set("filename", "custom-name.zip"))
	t.Cleanup(func() {
		_ = extensionPackageCmd.Flags().Set("output-directory", "")
		_ = extensionPackageCmd.Flags().Set("filename", "")
	})

	require.NoError(t, extensionPackageCmd.RunE(extensionPackageCmd, []string{extDir}))

	assert.FileExists(t, decoyBackup)
	assert.FileExists(t, decoyRelease)
	assert.FileExists(t, filepath.Join(outputDir, "custom-name.zip"))
}

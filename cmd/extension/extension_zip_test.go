package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestZipDoesNotDeleteUnrelatedZipsInWorkingDirectory(t *testing.T) {
	extDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(extDir, "composer.json"), []byte(`{
		"name": "frosh/frosh-test",
		"type": "shopware-platform-plugin",
		"license": "MIT",
		"version": "1.0.0",
		"require": { "shopware/core": "~6.6.0" },
		"autoload": { "psr-4": { "FroshTest\\": "src/" } },
		"extra": {
			"shopware-plugin-class": "FroshTest\\FroshTest",
			"label": { "de-DE": "Test", "en-GB": "Test" }
		}
	}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(extDir, ".shopware-extension.yml"), []byte(
		"build:\n  zip:\n    composer:\n      enabled: false\n    assets:\n      enabled: false\n",
	), 0o644))

	workDir := t.TempDir()
	decoyBackup := filepath.Join(workDir, "FroshTest-backup-2024.zip")
	decoyRelease := filepath.Join(workDir, "FroshTest-1.0.0.zip")
	require.NoError(t, os.WriteFile(decoyBackup, []byte("precious"), 0o644))
	require.NoError(t, os.WriteFile(decoyRelease, []byte("artifact"), 0o644))

	t.Chdir(workDir)

	disableGit = true
	t.Cleanup(func() { disableGit = false })

	outputDir := filepath.Join(workDir, "dist")
	extensionZipCmd.SetContext(t.Context())
	require.NoError(t, extensionZipCmd.Flags().Set("output-directory", outputDir))
	require.NoError(t, extensionZipCmd.Flags().Set("filename", "custom-name.zip"))
	t.Cleanup(func() {
		_ = extensionZipCmd.Flags().Set("output-directory", "")
		_ = extensionZipCmd.Flags().Set("filename", "")
	})

	require.NoError(t, extensionZipCmd.RunE(extensionZipCmd, []string{extDir}))

	assert.FileExists(t, decoyBackup)
	assert.FileExists(t, decoyRelease)
	assert.FileExists(t, filepath.Join(outputDir, "custom-name.zip"))
}

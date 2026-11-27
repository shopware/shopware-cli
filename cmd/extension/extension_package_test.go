package extension

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/extension"
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

func writeTestPlugin(t *testing.T) string {
	t.Helper()
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

	return extDir
}

func TestPackageOutsideGitRepositoryUsesWorkingCopy(t *testing.T) {
	extDir := writeTestPlugin(t)
	outputDir := t.TempDir()

	extensionPackageCmd.SetContext(t.Context())
	require.NoError(t, extensionPackageCmd.Flags().Set("output-directory", outputDir))
	t.Cleanup(func() {
		_ = extensionPackageCmd.Flags().Set("output-directory", "")
	})

	require.NoError(t, extensionPackageCmd.RunE(extensionPackageCmd, []string{extDir}))

	assert.FileExists(t, filepath.Join(outputDir, "FroshTest.zip"))
}

func TestPackageOutsideGitRepositoryWithGitCommitFails(t *testing.T) {
	extDir := writeTestPlugin(t)

	extensionPackageCmd.SetContext(t.Context())
	require.NoError(t, extensionPackageCmd.Flags().Set("git-commit", "1.0.0"))
	t.Cleanup(func() {
		_ = extensionPackageCmd.Flags().Set("git-commit", "")
	})

	err := extensionPackageCmd.RunE(extensionPackageCmd, []string{extDir})

	assert.ErrorIs(t, err, extension.ErrNotGitRepository)
}

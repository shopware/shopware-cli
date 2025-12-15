package extension

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func setupMockNPM(t *testing.T, tmpDir string) (string, string) {
	t.Helper()
	mockBinDir := t.TempDir()
	mockNpmPath := filepath.Join(mockBinDir, "npm")
	commandLogFile := filepath.Join(tmpDir, "npm-command.log")

	mockNpmScript := `#!/usr/bin/env sh
echo "$@" > "` + commandLogFile + `"
exit 0
`
	err := os.WriteFile(mockNpmPath, []byte(mockNpmScript), 0755)
	assert.NoError(t, err)

	return mockBinDir, commandLogFile
}

func TestInstallNPMDependencies_NpmCiLogic(t *testing.T) {
	t.Run("uses npm ci when package-lock.json exists", func(t *testing.T) {
		tmpDir := t.TempDir()

		mockBinDir, commandLogFile := setupMockNPM(t, tmpDir)

		packageJSON := `{"name": "test", "version": "1.0.0", "dependencies": {"test": "1.0.0"}}`
		err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644)
		assert.NoError(t, err)

		packageLock := `{"name": "test", "version": "1.0.0"}`
		err = os.WriteFile(filepath.Join(tmpDir, "package-lock.json"), []byte(packageLock), 0644)
		assert.NoError(t, err)

		packageData := NpmPackage{
			Dependencies: map[string]string{"test": "1.0.0"},
		}

		originalPath := os.Getenv("PATH")
		t.Setenv("PATH", mockBinDir+":"+originalPath)

		err = InstallNPMDependencies(context.Background(), tmpDir, packageData)
		assert.NoError(t, err)

		logContent, err := os.ReadFile(commandLogFile)
		assert.NoError(t, err)
		assert.Contains(t, string(logContent), "ci", "Expected npm ci command to be called when package-lock.json exists")
	})

	t.Run("uses npm install when package-lock.json missing", func(t *testing.T) {
		tmpDir := t.TempDir()

		mockBinDir, commandLogFile := setupMockNPM(t, tmpDir)

		packageJSON := `{"name": "test", "version": "1.0.0", "dependencies": {"test": "1.0.0"}}`
		err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644)
		assert.NoError(t, err)

		packageData := NpmPackage{
			Dependencies: map[string]string{"test": "1.0.0"},
		}

		originalPath := os.Getenv("PATH")
		t.Setenv("PATH", mockBinDir+":"+originalPath)

		err = InstallNPMDependencies(context.Background(), tmpDir, packageData)
		assert.NoError(t, err)

		logContent, err := os.ReadFile(commandLogFile)
		assert.NoError(t, err)
		assert.Contains(t, string(logContent), "install", "Expected npm install command to be called when package-lock.json is missing")
		assert.NotContains(t, string(logContent), "ci", "Should not use npm ci when package-lock.json is missing")
	})

	t.Run("skips installation when production mode and no dependencies", func(t *testing.T) {
		tmpDir := t.TempDir()

		packageJSON := `{"name": "test", "version": "1.0.0"}`
		err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644)
		assert.NoError(t, err)

		packageData := NpmPackage{
			Dependencies: map[string]string{},
		}

		err = InstallNPMDependencies(context.Background(), tmpDir, packageData, "--production")

		assert.NoError(t, err)
	})
}

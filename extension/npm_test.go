package extension

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstallNPMDependencies_NpmCiLogic(t *testing.T) {
	t.Run("uses npm ci when package-lock.json exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create a mock npm binary that records which command was called
		mockBinDir := t.TempDir()
		mockNpmPath := filepath.Join(mockBinDir, "npm")
		commandLogFile := filepath.Join(tmpDir, "npm-command.log")
		
		// Create a shell script that logs the command and exits successfully
		mockNpmScript := `#!/bin/sh
echo "$@" > "` + commandLogFile + `"
exit 0
`
		err := os.WriteFile(mockNpmPath, []byte(mockNpmScript), 0755)
		assert.NoError(t, err)

		// Create a minimal package.json
		packageJSON := `{"name": "test", "version": "1.0.0", "dependencies": {"test": "1.0.0"}}`
		err = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644)
		assert.NoError(t, err)

		// Create a package-lock.json
		packageLock := `{"name": "test", "version": "1.0.0"}`
		err = os.WriteFile(filepath.Join(tmpDir, "package-lock.json"), []byte(packageLock), 0644)
		assert.NoError(t, err)

		packageData := NpmPackage{
			Dependencies: map[string]string{"test": "1.0.0"},
		}

		// Set PATH to use our mock npm
		originalPath := os.Getenv("PATH")
		t.Setenv("PATH", mockBinDir+":"+originalPath)

		// Call InstallNPMDependencies
		err = InstallNPMDependencies(context.Background(), tmpDir, packageData)
		assert.NoError(t, err)

		// Read the command log to verify npm ci was called
		logContent, err := os.ReadFile(commandLogFile)
		assert.NoError(t, err)
		assert.Contains(t, string(logContent), "ci", "Expected npm ci command to be called when package-lock.json exists")
	})

	t.Run("uses npm install when package-lock.json missing", func(t *testing.T) {
		tmpDir := t.TempDir()
		
		// Create a mock npm binary that records which command was called
		mockBinDir := t.TempDir()
		mockNpmPath := filepath.Join(mockBinDir, "npm")
		commandLogFile := filepath.Join(tmpDir, "npm-command.log")
		
		// Create a shell script that logs the command and exits successfully
		mockNpmScript := `#!/bin/sh
echo "$@" > "` + commandLogFile + `"
exit 0
`
		err := os.WriteFile(mockNpmPath, []byte(mockNpmScript), 0755)
		assert.NoError(t, err)

		// Create a minimal package.json (no package-lock.json)
		packageJSON := `{"name": "test", "version": "1.0.0", "dependencies": {"test": "1.0.0"}}`
		err = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644)
		assert.NoError(t, err)

		packageData := NpmPackage{
			Dependencies: map[string]string{"test": "1.0.0"},
		}

		// Set PATH to use our mock npm
		originalPath := os.Getenv("PATH")
		t.Setenv("PATH", mockBinDir+":"+originalPath)

		// Call InstallNPMDependencies
		err = InstallNPMDependencies(context.Background(), tmpDir, packageData)
		assert.NoError(t, err)

		// Read the command log to verify npm install was called
		logContent, err := os.ReadFile(commandLogFile)
		assert.NoError(t, err)
		assert.Contains(t, string(logContent), "install", "Expected npm install command to be called when package-lock.json is missing")
		assert.NotContains(t, string(logContent), "ci", "Should not use npm ci when package-lock.json is missing")
	})

	t.Run("skips installation when production mode and no dependencies", func(t *testing.T) {
		tmpDir := t.TempDir()

		// Create a minimal package.json with no dependencies
		packageJSON := `{"name": "test", "version": "1.0.0"}`
		err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(packageJSON), 0644)
		assert.NoError(t, err)

		packageData := NpmPackage{
			Dependencies: map[string]string{},
		}

		// Should skip installation when in production mode with no dependencies
		err = InstallNPMDependencies(context.Background(), tmpDir, packageData, "--production")
		
		// Should return nil since it skips installation
		assert.NoError(t, err)
	})
}

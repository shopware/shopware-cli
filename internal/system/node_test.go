package system

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNodeVersionNotInstalled(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := GetInstalledNodeVersion(t.Context())
	assert.ErrorContains(t, err, "node.js is not installed")
}

func TestGetNodeVersion(t *testing.T) {
	tmpDir := t.TempDir()

	setupFakeNode(t, tmpDir, "18.16.0")

	nodeVersion, err := GetInstalledNodeVersion(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, "18.16.0", nodeVersion)
}

func TestNodeVersionIsAtLeast(t *testing.T) {
	setupFakeNode(t, t.TempDir(), "18.16.0")
	hit, err := IsNodeVersionAtLeast(t.Context(), "16.0.0")

	assert.NoError(t, err)
	assert.True(t, hit, "node.js version should be at least 16.0.0")
}

func TestNodeVersionIsNotAtLeast(t *testing.T) {
	setupFakeNode(t, t.TempDir(), "14.17.0")
	hit, err := IsNodeVersionAtLeast(t.Context(), "16.0.0")

	assert.NoError(t, err)
	assert.False(t, hit, "node.js version should not be at least 16.0.0")
}

func setupFakeNode(t *testing.T, tmpDir string, version string) {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	assert.NoError(t, err)

	// Node returns version with 'v' prefix
	assert.NoError(t, os.WriteFile(tmpDir+"/node", []byte(fmt.Sprintf("#!%s\necho v%s", shPath, version)), 0755))
	t.Setenv("PATH", tmpDir)
}

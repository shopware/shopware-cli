package system

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetPHPVersionNotInstalled(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := GetInstalledPHPVersion(t.Context())
	assert.ErrorContains(t, err, "PHP is not installed")
}

func TestGetPHPVersion(t *testing.T) {
	tmpDir := t.TempDir()

	setupFakePHP(t, tmpDir, "8.0.0")

	phpVersion, err := GetInstalledPHPVersion(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, "8.0.0", phpVersion)
}

func TestPHPVersionIsAtLeast(t *testing.T) {
	setupFakePHP(t, t.TempDir(), "8.0.0")
	hit, err := IsPHPVersionAtLeast(t.Context(), "8.0.0")

	assert.NoError(t, err)
	assert.True(t, hit, "PHP version should be at least 8.0.0")
}

func TestPHPVersionIsNotAtLeast(t *testing.T) {
	setupFakePHP(t, t.TempDir(), "7.4.0")
	hit, err := IsPHPVersionAtLeast(t.Context(), "8.0.0")

	assert.NoError(t, err)
	assert.False(t, hit, "PHP version should not be at least 8.0.0")
}

func TestGetAvailablePHPExtensionsNotInstalled(t *testing.T) {
	t.Setenv("PATH", "")
	_, err := GetAvailablePHPExtensions(t.Context())
	assert.ErrorContains(t, err, "PHP is not installed")
}

func TestGetAvailablePHPExtensions(t *testing.T) {
	tmpDir := t.TempDir()

	setupFakePHP(t, tmpDir, "8.0.0")

	extensions, err := GetAvailablePHPExtensions(t.Context())
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"Core", "curl", "json", "mbstring"}, extensions)
}

func setupFakePHP(t *testing.T, tmpDir string, version string) {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	assert.NoError(t, err)

	script := fmt.Sprintf(`#!%s
if [ "$1" = "-m" ]; then
  printf '[PHP Modules]\nCore\ncurl\njson\nmbstring\n\n[Zend Modules]\n'
else
  echo PHP %s
fi
`, shPath, version)

	assert.NoError(t, os.WriteFile(tmpDir+"/php", []byte(script), 0755))
	t.Setenv("PATH", tmpDir)
}

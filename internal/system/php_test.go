package system

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPHPVersionNotInstalled(t *testing.T) {
	t.Setenv("PHP_BINARY", "")
	t.Setenv("PATH", "")
	_, err := GetInstalledPHPVersion(t.Context())
	assert.ErrorContains(t, err, "cannot find PHP installation")
}

func TestGetPHPVersionPrefersPHPBinaryEnv(t *testing.T) {
	pathDir := t.TempDir()
	writeFakePHP(t, pathDir+"/php", "8.0.0")

	binDir := t.TempDir()
	phpBinary := binDir + "/php"
	writeFakePHP(t, phpBinary, "8.2.0")

	t.Setenv("PATH", pathDir)
	t.Setenv("PHP_BINARY", phpBinary)

	phpVersion, err := GetInstalledPHPVersion(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, "8.2.0", phpVersion)
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
	t.Setenv("PHP_BINARY", "")
	t.Setenv("PATH", "")
	_, err := GetAvailablePHPExtensions(t.Context())
	assert.ErrorContains(t, err, "cannot find PHP installation")
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
	t.Setenv("PHP_BINARY", "")
	writeFakePHP(t, tmpDir+"/php", version)
	t.Setenv("PATH", tmpDir)
}

func writeFakePHP(t *testing.T, path string, version string) {
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

	assert.NoError(t, os.WriteFile(path, []byte(script), 0755))
}

// writeFakePHPWithStartupWarning emits a startup warning before the banner, as
// PHP does when php.ini references a missing extension.
func writeFakePHPWithStartupWarning(t *testing.T, path string, version string) {
	t.Helper()
	shPath, err := exec.LookPath("sh")
	assert.NoError(t, err)

	script := fmt.Sprintf(`#!%s
echo "Warning: PHP Startup: Unable to load dynamic library 'gone.so'"
echo PHP %s
`, shPath, version)

	assert.NoError(t, os.WriteFile(path, []byte(script), 0o755))
}

func TestGetPHPVersionOfBinaryIgnoresStartupWarnings(t *testing.T) {
	dir := t.TempDir()
	writeFakePHPWithStartupWarning(t, dir+"/php", "8.5.9")

	phpVersion, err := GetPHPVersionOfBinary(t.Context(), dir+"/php")
	assert.NoError(t, err)
	assert.Equal(t, "8.5.9", phpVersion)
}

func TestGetPHPVersionOfBinaryStopsChildProcessesOnCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix process groups")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "php")
	ready := filepath.Join(dir, "child-ready")
	release := filepath.Join(dir, "release")
	marker := filepath.Join(dir, "child-finished")
	script := fmt.Sprintf(`#!/bin/sh
(
  : > %s
  while [ ! -f %s ]; do sleep 0.01; done
  : > %s
) &
wait
`, strconv.Quote(ready), strconv.Quote(release), strconv.Quote(marker))
	require.NoError(t, os.WriteFile(binary, []byte(script), 0o755))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); _ = os.WriteFile(release, nil, 0o644) })
	done := make(chan error, 1)
	go func() { _, err := GetPHPVersionOfBinary(ctx, binary); done <- err }()
	require.Eventually(t, func() bool { _, err := os.Stat(ready); return err == nil }, 5*time.Second, 10*time.Millisecond)

	cancel()
	select {
	case err := <-done:
		assert.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("version probe did not stop")
	}

	require.NoError(t, os.WriteFile(release, nil, 0o644))
	assert.Never(t, func() bool { _, err := os.Stat(marker); return err == nil }, time.Second, 10*time.Millisecond)
}

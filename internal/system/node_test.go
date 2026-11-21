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

func TestGetInstalledNodeVersionStopsChildProcessesOnCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires Unix process groups")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "node")
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
	go func() { _, err := GetInstalledNodeVersion(ctx); done <- err }()
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

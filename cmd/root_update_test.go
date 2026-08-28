package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/update"
)

func TestShouldNotifySuppressesRecentHomebrewRelease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake brew shell script requires a unix shell")
	}

	prefix := t.TempDir()
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "brew"), []byte("#!/bin/sh\necho "+prefix+"\n"), 0o755))
	t.Setenv("PATH", binDir)

	recent := &update.ReleaseInfo{Version: "99.0.0", PublishedAt: time.Now()}
	old := &update.ReleaseInfo{Version: "99.0.0", PublishedAt: time.Now().Add(-48 * time.Hour)}
	homebrewBinary := filepath.Join(prefix, "bin", "shopware-cli")

	assert.False(t, shouldNotify(recent, homebrewBinary))
	assert.True(t, shouldNotify(recent, "/usr/local/other/shopware-cli"))
	assert.True(t, shouldNotify(old, homebrewBinary))
}

package update

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShouldNotify(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake brew shell script requires a unix shell")
	}

	prefix := t.TempDir()
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "brew"), []byte("#!/bin/sh\necho "+prefix+"\n"), 0o755))
	t.Setenv("PATH", binDir)

	recent := &ReleaseInfo{Version: "99.0.0", PublishedAt: time.Now()}
	old := &ReleaseInfo{Version: "99.0.0", PublishedAt: time.Now().Add(-48 * time.Hour)}
	homebrewBinary := filepath.Join(prefix, "bin", "shopware-cli")

	assert.False(t, ShouldNotify(recent, homebrewBinary))
	assert.True(t, ShouldNotify(recent, "/usr/local/other/shopware-cli"))
	assert.True(t, ShouldNotify(old, homebrewBinary))
}

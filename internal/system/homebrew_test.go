package system

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsUnderHomebrew(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake brew shell script requires a unix shell")
	}

	prefix := t.TempDir()
	binDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "brew"), []byte("#!/bin/sh\necho "+prefix+"\n"), 0o755))
	t.Setenv("PATH", binDir)

	assert.True(t, IsUnderHomebrew(filepath.Join(prefix, "bin", "shopware-cli")))
	assert.False(t, IsUnderHomebrew("/usr/local/bin/shopware-cli"))
}

func TestIsUnderHomebrewReturnsFalseWithoutBrew(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	assert.False(t, IsUnderHomebrew("/usr/local/bin/shopware-cli"))
}

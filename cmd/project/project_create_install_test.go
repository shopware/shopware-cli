package project

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFakeBinary(t *testing.T, dir, name, script string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755))
}

func TestRunComposerInstallLocalComposerBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake binaries are shell scripts")
	}

	binDir := t.TempDir()
	writeFakeBinary(t, binDir, "composer", "#!/bin/sh\necho composer-ran in $PWD\nexit ${FAKE_COMPOSER_EXIT:-0}\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PHP_BINARY", "")

	projectDir := t.TempDir()

	t.Run("composer from PATH runs in the project folder", func(t *testing.T) {
		out, err := runComposerInstall(t.Context(), projectDir, false, false, "", "")
		require.NoError(t, err)
		assert.Contains(t, out, "composer-ran")
		assert.Contains(t, out, projectDir)
	})

	t.Run("explicit php binary wraps composer", func(t *testing.T) {
		writeFakeBinary(t, binDir, "fake-php", "#!/bin/sh\necho php-wrapper \"$@\"\n")
		out, err := runComposerInstall(t.Context(), projectDir, false, false, "", filepath.Join(binDir, "fake-php"))
		require.NoError(t, err)
		assert.Contains(t, out, "php-wrapper")
		assert.Contains(t, out, "install")
	})

	// The captured output on failure feeds the security-blocked retry logic.
	t.Run("failure returns error and captured output", func(t *testing.T) {
		t.Setenv("FAKE_COMPOSER_EXIT", "3")
		out, err := runComposerInstall(t.Context(), projectDir, false, false, "", "")
		require.Error(t, err)
		assert.Contains(t, out, "composer-ran")
	})
}

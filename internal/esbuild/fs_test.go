package esbuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyStaticFiles(t *testing.T) {
	t.Run("copies nested files and permissions", func(t *testing.T) {
		sourceDir := t.TempDir()
		targetDir := filepath.Join(t.TempDir(), "target")
		sourceFile := filepath.Join(sourceDir, "images", "icon.svg")
		require.NoError(t, os.MkdirAll(filepath.Dir(sourceFile), 0o755))
		require.NoError(t, os.WriteFile(sourceFile, []byte("<svg>icon</svg>"), 0o640))

		require.NoError(t, copyStaticFiles(sourceDir, targetDir))

		targetFile := filepath.Join(targetDir, "images", "icon.svg")
		contents, err := os.ReadFile(targetFile)
		require.NoError(t, err)
		assert.Equal(t, "<svg>icon</svg>", string(contents))

		info, err := os.Stat(targetFile)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	})

	t.Run("ignores missing source directory", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "target")

		require.NoError(t, copyStaticFiles(filepath.Join(t.TempDir(), "missing"), targetDir))

		_, err := os.Stat(targetDir)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("reports target directory creation failure", func(t *testing.T) {
		sourceDir := t.TempDir()
		targetParent := filepath.Join(t.TempDir(), "target")
		require.NoError(t, os.WriteFile(targetParent, []byte("file"), 0o644))

		err := copyStaticFiles(sourceDir, filepath.Join(targetParent, "nested"))

		assert.ErrorContains(t, err, "failed to create target directory")
	})
}

func TestCopyFileErrors(t *testing.T) {
	t.Run("missing source", func(t *testing.T) {
		err := copyFile(filepath.Join(t.TempDir(), "missing"), filepath.Join(t.TempDir(), "target"))

		assert.ErrorContains(t, err, "failed to open source file")
	})

	t.Run("missing target parent", func(t *testing.T) {
		sourceFile := filepath.Join(t.TempDir(), "source")
		require.NoError(t, os.WriteFile(sourceFile, []byte("source"), 0o644))

		err := copyFile(sourceFile, filepath.Join(t.TempDir(), "missing", "target"))

		assert.ErrorContains(t, err, "failed to create target file")
	})
}

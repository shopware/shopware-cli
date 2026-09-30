package system

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFiles(t *testing.T) {
	t.Parallel()
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	defer func() {
		err := os.RemoveAll(tempDir)
		assert.NoError(t, err, "Failed to remove temporary directory")
	}()

	// Create source directory structure
	srcDir := filepath.Join(tempDir, "src")
	err := os.MkdirAll(srcDir, 0o755)
	assert.NoError(t, err, "Failed to create source directory")

	// Create a normal file
	normalFile := filepath.Join(srcDir, "normal.txt")
	err = os.WriteFile(normalFile, []byte("normal content"), 0o644)
	assert.NoError(t, err, "Failed to create normal file")

	// Create a .devenv directory with a file
	devenvDir := filepath.Join(srcDir, ".devenv")
	err = os.MkdirAll(devenvDir, 0o755)
	assert.NoError(t, err, "Failed to create .devenv directory")
	devenvFile := filepath.Join(devenvDir, "devenv.txt")
	err = os.WriteFile(devenvFile, []byte("devenv content"), 0o644)
	assert.NoError(t, err, "Failed to create file in .devenv")

	// Create a .direnv directory with a file
	direnvDir := filepath.Join(srcDir, ".direnv")
	err = os.MkdirAll(direnvDir, 0o755)
	assert.NoError(t, err, "Failed to create .direnv directory")
	direnvFile := filepath.Join(direnvDir, "direnv.txt")
	err = os.WriteFile(direnvFile, []byte("direnv content"), 0o644)
	assert.NoError(t, err, "Failed to create file in .direnv")

	// Create a regular subdirectory with a file
	subDir := filepath.Join(srcDir, "subdir")
	err = os.MkdirAll(subDir, 0o755)
	assert.NoError(t, err, "Failed to create subdirectory")
	subFile := filepath.Join(subDir, "sub.txt")
	err = os.WriteFile(subFile, []byte("sub content"), 0o644)
	assert.NoError(t, err, "Failed to create file in subdirectory")

	// Create nested Git metadata with a file that must not be copied
	nestedGitDir := filepath.Join(srcDir, "custom", "plugins", "example", ".git")
	err = os.MkdirAll(nestedGitDir, 0o755)
	assert.NoError(t, err, "Failed to create nested .git directory")
	err = os.WriteFile(filepath.Join(nestedGitDir, "metadata"), []byte("git metadata"), 0o644)
	assert.NoError(t, err, "Failed to create nested .git file")

	// Create destination directory
	dstDir := filepath.Join(tempDir, "dst")

	// Copy files from src to dst
	err = CopyFiles(t.Context(), srcDir, dstDir)
	assert.NoError(t, err, "copyFiles failed")

	// Check if normal file was copied
	dstNormalFile := filepath.Join(dstDir, "normal.txt")
	_, err = os.Stat(dstNormalFile)
	assert.False(t, os.IsNotExist(err), "Normal file was not copied")

	// Check if file in subdirectory was copied
	dstSubFile := filepath.Join(dstDir, "subdir", "sub.txt")
	_, err = os.Stat(dstSubFile)
	assert.False(t, os.IsNotExist(err), "File in subdirectory was not copied")

	// Check if .devenv directory was excluded
	dstDevenvDir := filepath.Join(dstDir, ".devenv")
	_, err = os.Stat(dstDevenvDir)
	assert.True(t, os.IsNotExist(err), ".devenv directory was not excluded")

	// Check if .direnv directory was excluded
	dstDirenvDir := filepath.Join(dstDir, ".direnv")
	_, err = os.Stat(dstDirenvDir)
	assert.True(t, os.IsNotExist(err), ".direnv directory was not excluded")

	// Check nested .git directory was excluded
	dstNestedGitDir := filepath.Join(dstDir, "custom", "plugins", "example", ".git")
	_, err = os.Stat(dstNestedGitDir)
	assert.True(t, os.IsNotExist(err), "nested .git directory was not excluded")
}

func TestIsDirEmpty(t *testing.T) {
	t.Parallel()
	// Test empty directory
	tmpDir := t.TempDir()
	empty, err := IsDirEmpty(tmpDir)
	assert.NoError(t, err)
	assert.True(t, empty)

	// Test non-empty directory
	f, err := os.Create(filepath.Join(tmpDir, "test"))
	assert.NoError(t, err)
	err = f.Close()
	assert.NoError(t, err)

	empty, err = IsDirEmpty(tmpDir)
	assert.NoError(t, err)
	assert.False(t, empty)
}

func TestCopyFilesTree(t *testing.T) {
	t.Parallel()
	src := t.TempDir()
	dst := t.TempDir()
	for _, dir := range []string{".git", ".devenv", ".direnv", "nested/.git", "nested/.devenv", "empty"} {
		require.NoError(t, os.MkdirAll(filepath.Join(src, dir), 0o755))
	}
	for i := range 100 {
		name := filepath.Join("nested", strconv.Itoa(i))
		require.NoError(t, os.WriteFile(filepath.Join(src, name), []byte(name), 0o640))
	}
	require.NoError(t, os.WriteFile(filepath.Join(src, "nested/.git/keep"), []byte("nested metadata"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "unrelated"), []byte("keep"), 0o644))

	// Copying twice exercises merging existing directories and overwriting files.
	for range 2 {
		require.NoError(t, CopyFiles(t.Context(), src, dst))
	}
	for i := range 100 {
		name := filepath.Join("nested", strconv.Itoa(i))
		content, err := os.ReadFile(filepath.Join(dst, name))
		require.NoError(t, err)
		assert.Equal(t, name, string(content))
	}
	for _, name := range []string{".git", ".devenv", ".direnv", "nested/.git", "nested/.devenv"} {
		_, err := os.Lstat(filepath.Join(dst, name))
		assert.ErrorIs(t, err, os.ErrNotExist)
	}
	assert.FileExists(t, filepath.Join(dst, "unrelated"))
	assert.DirExists(t, filepath.Join(dst, "empty"))
}

func TestCopyFilesSourceAndTarget(t *testing.T) {
	t.Parallel()
	t.Run("missing source", func(t *testing.T) {
		t.Parallel()
		base := t.TempDir()
		target := filepath.Join(base, "target")
		require.NoError(t, CopyFiles(t.Context(), filepath.Join(base, "missing"), target))
		assert.NoDirExists(t, target)
	})
	t.Run("relative source", func(t *testing.T) {
		t.Parallel()
		src := t.TempDir()
		wd, err := os.Getwd()
		require.NoError(t, err)
		rel, err := filepath.Rel(wd, src)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("source"), 0o644))
		dst := t.TempDir()
		require.NoError(t, CopyFiles(t.Context(), rel, dst))
		assert.FileExists(t, filepath.Join(dst, "file"))
	})
	t.Run("same directory", func(t *testing.T) {
		t.Parallel()
		src := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("source"), 0o644))
		require.Error(t, CopyFiles(t.Context(), src, src))
		content, err := os.ReadFile(filepath.Join(src, "file"))
		require.NoError(t, err)
		assert.Equal(t, "source", string(content))
	})
	t.Run("target inside source", func(t *testing.T) {
		t.Parallel()
		src := t.TempDir()
		dst := filepath.Join(src, "nested", "copy")
		require.ErrorContains(t, CopyFiles(t.Context(), src, dst), "must not be inside source directory")
		assert.NoDirExists(t, dst)
	})
	t.Run("source named like a skipped directory", func(t *testing.T) {
		t.Parallel()
		src := filepath.Join(t.TempDir(), ".git")
		require.NoError(t, os.Mkdir(src, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(src, "HEAD"), []byte("ref"), 0o644))
		dst := t.TempDir()
		require.NoError(t, CopyFiles(t.Context(), src, dst))
		assert.FileExists(t, filepath.Join(dst, "HEAD"))
	})
	t.Run("file source", func(t *testing.T) {
		t.Parallel()
		src := filepath.Join(t.TempDir(), "file")
		require.NoError(t, os.WriteFile(src, []byte("source"), 0o644))
		assert.ErrorContains(t, CopyFiles(t.Context(), src, t.TempDir()), "not a directory")
	})
	t.Run("cancelled context", func(t *testing.T) {
		t.Parallel()
		src, dst := t.TempDir(), t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("source"), 0o644))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, CopyFiles(ctx, src, dst), context.Canceled)
		assert.NoFileExists(t, filepath.Join(dst, "file"))
	})
	t.Run("worker failure", func(t *testing.T) {
		t.Parallel()
		src, dst := t.TempDir(), t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("source"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(dst, "file"), 0o755))
		err := CopyFiles(t.Context(), src, dst)
		assert.ErrorContains(t, err, "cannot overwrite directory")
		assert.NotErrorIs(t, err, context.Canceled)
		assert.DirExists(t, filepath.Join(dst, "file"))
	})
	t.Run("walk failure", func(t *testing.T) {
		t.Parallel()
		src, dst := t.TempDir(), t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(src, "directory"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dst, "directory"), []byte("keep"), 0o644))
		require.Error(t, CopyFiles(t.Context(), src, dst))
		content, err := os.ReadFile(filepath.Join(dst, "directory"))
		require.NoError(t, err)
		assert.Equal(t, "keep", string(content))
	})
}

func TestCopyFileContentsAndPermissions(t *testing.T) {
	t.Parallel()
	for name, copyFn := range map[string]func(context.Context, string, string, fs.FileInfo) error{
		"automatic": copyFile,
		"fallback":  copyFileFallback,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			for _, mode := range []fs.FileMode{0o640, 0o755, 0o400, 0o755 | os.ModeSetuid | os.ModeSetgid} {
				t.Run(mode.String(), func(t *testing.T) {
					t.Parallel()
					if runtime.GOOS == "windows" {
						t.Skip("Unix file permissions")
					}
					base := t.TempDir()
					src, dst := filepath.Join(base, "source"), filepath.Join(base, "target")
					original := strings.Repeat("original contents", 8192)
					require.NoError(t, os.WriteFile(src, []byte(original), 0o600))
					require.NoError(t, os.Chmod(src, mode))
					info, err := os.Stat(src)
					require.NoError(t, err)

					for range 2 {
						require.NoError(t, copyFn(t.Context(), src, dst, info))
						content, err := os.ReadFile(dst)
						require.NoError(t, err)
						assert.Equal(t, original, string(content))
						targetInfo, err := os.Stat(dst)
						require.NoError(t, err)
						assert.Equal(t, info.Mode(), targetInfo.Mode())
					}
					require.NoError(t, os.Chmod(dst, 0o600))
					require.NoError(t, os.WriteFile(dst, []byte("changed copy"), 0o600))
					content, err := os.ReadFile(src)
					require.NoError(t, err)
					assert.Equal(t, original, string(content), "copy must not share writes with source")
				})
			}
		})
	}
}

func TestCopyFileExistingDestinations(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlinks and hard links")
	}
	for name, copyFn := range map[string]func(context.Context, string, string, fs.FileInfo) error{
		"automatic": copyFile,
		"fallback":  copyFileFallback,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			src, dst, other := filepath.Join(base, "source"), filepath.Join(base, "target"), filepath.Join(base, "other")
			require.NoError(t, os.WriteFile(src, []byte("source"), 0o644))
			require.NoError(t, os.WriteFile(other, []byte("keep"), 0o644))
			info, err := os.Stat(src)
			require.NoError(t, err)

			require.Error(t, copyFn(t.Context(), src, src, info))
			require.NoError(t, os.Link(src, dst))
			require.Error(t, copyFn(t.Context(), src, dst, info))
			content, err := os.ReadFile(src)
			require.NoError(t, err)
			assert.Equal(t, "source", string(content))
			require.NoError(t, os.Remove(dst))

			require.NoError(t, os.Symlink(other, dst))
			require.NoError(t, copyFn(t.Context(), src, dst, info))
			content, err = os.ReadFile(other)
			require.NoError(t, err)
			assert.Equal(t, "keep", string(content), "must replace destination symlink without following it")
			content, err = os.ReadFile(dst)
			require.NoError(t, err)
			assert.Equal(t, "source", string(content))
			require.NoError(t, os.Remove(dst))

			require.NoError(t, os.Mkdir(dst, 0o755))
			require.Error(t, copyFn(t.Context(), src, dst, info))
			assert.DirExists(t, dst)
		})
	}
}

func TestCopyFileFallbackCanceled(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	src, dst := filepath.Join(base, "source"), filepath.Join(base, "target")
	require.NoError(t, os.WriteFile(src, []byte("source"), 0o644))
	info, err := os.Stat(src)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	assert.ErrorIs(t, copyFileFallback(ctx, src, dst, info), context.Canceled)
}

func TestCopyFilesSymlinks(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Unix symlinks")
	}
	base := t.TempDir()
	src, dst := filepath.Join(base, "source"), filepath.Join(base, "target")
	require.NoError(t, os.Mkdir(src, 0o755))
	require.NoError(t, os.Mkdir(dst, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("content"), 0o644))
	for name, target := range map[string]string{"relative": "file", "dangling": "missing", "directory": "."} {
		require.NoError(t, os.Symlink(target, filepath.Join(src, name)))
		require.NoError(t, os.WriteFile(filepath.Join(dst, name), []byte("old"), 0o644))
	}
	alias := filepath.Join(base, "alias")
	require.NoError(t, os.Symlink(src, alias))
	require.Error(t, CopyFiles(t.Context(), src, alias))
	require.Error(t, CopyFiles(t.Context(), src, filepath.Join(alias, "nested")))
	assert.NoDirExists(t, filepath.Join(src, "nested"))
	// Source directory aliases work, while symlinks within the tree stay links.
	for range 2 {
		require.NoError(t, CopyFiles(t.Context(), alias, dst))
	}
	for name, target := range map[string]string{"relative": "file", "dangling": "missing", "directory": "."} {
		actual, err := os.Readlink(filepath.Join(dst, name))
		require.NoError(t, err)
		assert.Equal(t, target, actual)
	}
}

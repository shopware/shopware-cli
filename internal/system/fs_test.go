package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCopyFiles(t *testing.T) {
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

	// Create destination directory
	dstDir := filepath.Join(tempDir, "dst")

	// Copy files from src to dst
	err = CopyFiles(srcDir, dstDir)
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
}

func TestIsDirEmpty(t *testing.T) {
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

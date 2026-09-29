//go:build unix

package system

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyFilesNamedPipe(t *testing.T) {
	t.Parallel()
	src, dst := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(src, "file"), []byte("content"), 0o644))
	require.NoError(t, syscall.Mkfifo(filepath.Join(src, "pipe"), 0o644))

	err := CopyFiles(t.Context(), src, dst)
	assert.ErrorContains(t, err, "unsupported file type: named pipe")
	assert.FileExists(t, filepath.Join(dst, "file"))
}

//go:build unix

package extension

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetExtensionByZipRejectsFIFO(t *testing.T) {
	name := filepath.Join(t.TempDir(), "archive.zip")
	require.NoError(t, syscall.Mkfifo(name, 0o600))
	done := make(chan error, 1)
	go func() { _, err := GetExtensionByZip(t.Context(), name); done <- err }()
	select {
	case err := <-done:
		assert.Error(t, err)
	case <-time.After(time.Second):
		t.Error("archive discovery blocked opening FIFO")
		file, err := os.OpenFile(name, os.O_RDWR, 0)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		assert.Error(t, <-done)
	}
}

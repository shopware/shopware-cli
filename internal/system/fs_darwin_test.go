//go:build darwin

package system

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestCloneFile(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	src, dst := filepath.Join(base, "source"), filepath.Join(base, "target")
	require.NoError(t, os.WriteFile(src, []byte("original"), 0o644))
	err := cloneFile(src, dst)
	if errors.Is(err, unix.ENOTSUP) {
		t.Skip("test filesystem does not support cloning")
	}
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(dst, []byte("changed"), 0o644))
	content, err := os.ReadFile(src)
	require.NoError(t, err)
	assert.Equal(t, "original", string(content))

	// An existing destination must survive a failed clone attempt.
	require.ErrorIs(t, cloneFile(src, dst), os.ErrExist)
	content, err = os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "changed", string(content))
}

package system

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnsureWritableHomeKeepsWritableHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("EnsureWritableHome only acts on Linux")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)

	assert.Equal(t, "", EnsureWritableHome())
	assert.Equal(t, home, os.Getenv("HOME"))
}

func TestEnsureWritableHomeRedirectsUnsetHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("EnsureWritableHome only acts on Linux")
	}

	t.Setenv("HOME", "")

	redirected := EnsureWritableHome()
	assert.Equal(t, os.TempDir(), redirected)
	assert.Equal(t, os.TempDir(), os.Getenv("HOME"))
}

func TestEnsureWritableHomeRedirectsUnwritableHome(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("EnsureWritableHome only acts on Linux")
	}

	// A path that does not exist is unwritable even for root, keeping the test
	// deterministic in the root-mapped CI sandbox.
	t.Setenv("HOME", filepath.Join(t.TempDir(), "does-not-exist"))

	redirected := EnsureWritableHome()
	assert.Equal(t, os.TempDir(), redirected)
	assert.Equal(t, os.TempDir(), os.Getenv("HOME"))
}

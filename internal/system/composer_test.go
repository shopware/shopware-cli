package system

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stubComposerPharServer serves a fake Composer PHAR (and its checksum file)
// and points composerPharURL at it. The checksum can be tampered with via
// breakChecksum to simulate a corrupted download.
func stubComposerPharServer(t *testing.T, pharContent string, breakChecksum bool) {
	t.Helper()

	sum := sha256.Sum256([]byte(pharContent))
	checksum := hex.EncodeToString(sum[:])
	if breakChecksum {
		checksum = "0000000000000000000000000000000000000000000000000000000000000000"
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/composer.phar":
			_, _ = w.Write([]byte(pharContent))
		case "/composer.phar.sha256sum":
			_, _ = fmt.Fprintf(w, "%s  composer.phar\n", checksum)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	original := composerPharURL
	composerPharURL = server.URL + "/composer.phar"
	t.Cleanup(func() { composerPharURL = original })
}

func TestResolveComposerPrefersPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake composer executable requires a shell script")
	}

	binDir := t.TempDir()
	composerPath := filepath.Join(binDir, "composer")
	assert.NoError(t, os.WriteFile(composerPath, []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", binDir)
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	path, isPhar, err := ResolveComposer(t.Context())
	assert.NoError(t, err)
	assert.False(t, isPhar)
	assert.Equal(t, composerPath, path)
}

func TestResolveComposerDownloadsPhar(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cacheDir := t.TempDir()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", cacheDir)
	stubComposerPharServer(t, "fake composer phar", false)

	path, isPhar, err := ResolveComposer(t.Context())
	assert.NoError(t, err)
	assert.True(t, isPhar)
	assert.Equal(t, filepath.Join(cacheDir, "composer.phar"), path)

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "fake composer phar", string(content))

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		assert.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
	}
}

func TestResolveComposerReusesCachedPhar(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cacheDir := t.TempDir()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", cacheDir)
	assert.NoError(t, os.WriteFile(filepath.Join(cacheDir, "composer.phar"), []byte("cached phar"), 0o755))

	// No stub server: any download attempt would hit getcomposer.org and fail
	// the test on the unexpected network call being slow or blocked; the cached
	// PHAR must short-circuit before that.
	path, isPhar, err := ResolveComposer(t.Context())
	assert.NoError(t, err)
	assert.True(t, isPhar)
	assert.Equal(t, filepath.Join(cacheDir, "composer.phar"), path)

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "cached phar", string(content))
}

func TestResolveComposerRejectsCorruptedDownload(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cacheDir := t.TempDir()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", cacheDir)
	stubComposerPharServer(t, "fake composer phar", true)

	_, _, err := ResolveComposer(t.Context())
	assert.ErrorContains(t, err, "composer download is corrupted")

	_, statErr := os.Stat(filepath.Join(cacheDir, "composer.phar"))
	assert.True(t, os.IsNotExist(statErr), "a corrupted PHAR must not be cached")
}

func TestResolveComposerReportsDownloadFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())

	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	original := composerPharURL
	composerPharURL = server.URL + "/composer.phar"
	t.Cleanup(func() { composerPharURL = original })

	_, _, err := ResolveComposer(t.Context())
	assert.ErrorContains(t, err, "cannot download")
}

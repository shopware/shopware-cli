package system

import (
	"context"
	"crypto/sha512"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposerResolverUsesPathComposerWithoutDownload(t *testing.T) {
	t.Parallel()

	resolver := &ComposerResolver{
		LookPath:         func(string) (string, error) { return "/usr/bin/composer", nil },
		ResolvePHPBinary: func() (string, error) { return "", assert.AnError },
	}

	composer, err := resolver.Resolve(t.Context())

	require.NoError(t, err)
	assert.Equal(t, "/usr/bin/composer", composer.Path)
	assert.False(t, composer.Temporary)
}

func TestComposerResolverBootstrapsVerifiedComposer(t *testing.T) {
	installer := []byte("verified Composer installer")
	signature := fmt.Sprintf("%x", sha512.Sum384(installer))
	server := newComposerDownloadServer(t, installer, signature, http.StatusOK)

	var installerPHP, installerPath string
	resolver := newTestComposerResolver(t, server, func(_ context.Context, php, installer, tempDir string) error {
		installerPHP = php
		installerPath = installer
		return os.WriteFile(filepath.Join(tempDir, "composer.phar"), []byte("phar"), 0o600)
	})

	composer, err := resolver.Resolve(t.Context())

	require.NoError(t, err)
	assert.True(t, composer.Temporary)
	assert.Equal(t, "/selected/php", composer.PHPBinary)
	assert.Equal(t, "/selected/php", installerPHP)
	assert.Equal(t, filepath.Join(filepath.Dir(composer.Path), "composer-setup.php"), installerPath)
	assert.FileExists(t, composer.Path)
	assert.NoFileExists(t, installerPath)
	tempDir := filepath.Dir(composer.Path)
	composer.Cleanup()
	assert.NoDirExists(t, tempDir)
}

func TestComposerResolverRejectsInvalidDownloadsAndCleansUp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		signature string
	}{
		{name: "unexpected status", status: http.StatusBadGateway, signature: ""},
		{name: "malformed signature", status: http.StatusOK, signature: "not-a-checksum"},
		{name: "checksum mismatch", status: http.StatusOK, signature: fmt.Sprintf("%x", sha512.Sum384([]byte("different")))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newComposerDownloadServer(t, []byte("installer"), test.signature, test.status)
			tempRoot := t.TempDir()
			resolver := newTestComposerResolver(t, server, func(context.Context, string, string, string) error {
				t.Fatal("installer must not run for an invalid download")
				return nil
			})
			resolver.TempDir = tempRoot

			composer, err := resolver.Resolve(t.Context())

			assert.Nil(t, composer)
			assert.ErrorContains(t, err, "temporary Composer download failed")
			entries, readErr := os.ReadDir(tempRoot)
			require.NoError(t, readErr)
			assert.Empty(t, entries)
		})
	}
}

func TestComposerResolverCleansUpAfterInstallerFailure(t *testing.T) {
	t.Parallel()

	installer := []byte("verified Composer installer")
	server := newComposerDownloadServer(t, installer, fmt.Sprintf("%x", sha512.Sum384(installer)), http.StatusOK)
	tempRoot := t.TempDir()
	resolver := newTestComposerResolver(t, server, func(context.Context, string, string, string) error {
		return errors.New("installer failed")
	})
	resolver.TempDir = tempRoot

	composer, err := resolver.Resolve(t.Context())

	assert.Nil(t, composer)
	assert.ErrorContains(t, err, "installer failed")
	entries, readErr := os.ReadDir(tempRoot)
	require.NoError(t, readErr)
	assert.Empty(t, entries)
}

func TestComposerResolverHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	resolver := newTestComposerResolver(t, server, nil)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	composer, err := resolver.Resolve(ctx)

	assert.Nil(t, composer)
	assert.ErrorContains(t, err, "temporary Composer download failed")
}

func TestComposerCommandUsesResolvedPHPForPHAR(t *testing.T) {
	t.Parallel()

	composer := &Composer{Path: "/temporary/composer.phar", PHPBinary: "/selected/php"}
	cmd := composer.Command(t.Context(), "install", "--no-interaction")

	assert.Equal(t, "/selected/php", cmd.Path)
	assert.Equal(t, []string{"/selected/php", "/temporary/composer.phar", "install", "--no-interaction"}, cmd.Args)
}

func newComposerDownloadServer(t *testing.T, installer []byte, signature string, status int) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/installer" {
			w.WriteHeader(status)
			_, _ = w.Write(installer)
			return
		}
		if r.URL.Path == "/installer.sig" {
			_, _ = w.Write([]byte(signature))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func newTestComposerResolver(t *testing.T, server *httptest.Server, runInstaller func(context.Context, string, string, string) error) *ComposerResolver {
	t.Helper()
	return &ComposerResolver{
		HTTPClient:            server.Client(),
		InstallerURL:          server.URL + "/installer",
		InstallerSignatureURL: server.URL + "/installer.sig",
		LookPath:              func(string) (string, error) { return "", errors.New("not found") },
		ResolvePHPBinary:      func() (string, error) { return "/selected/php", nil },
		RunInstaller:          runInstaller,
	}
}

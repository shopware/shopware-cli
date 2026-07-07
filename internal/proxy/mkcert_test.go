package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// installFakeMkcert puts a fake mkcert script on PATH that echoes the CAROOT
// and records certificate generation calls.
func installFakeMkcert(t *testing.T, caroot string) string {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("fake mkcert script is not supported on windows")
	}

	binDir := t.TempDir()
	argsFile := filepath.Join(binDir, "args.txt")

	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "-CAROOT" ]; then
  echo %q
  exit 0
fi
echo "$@" > %q
# emulate certificate generation: -cert-file <path> -key-file <path> hosts...
: > "$2"
: > "$4"
`, caroot, argsFile)

	assert.NoError(t, os.WriteFile(filepath.Join(binDir, "mkcert"), []byte(script), 0o755))
	t.Setenv("PATH", binDir)

	return argsFile
}

func TestMkcertAvailable(t *testing.T) {
	installFakeMkcert(t, t.TempDir())
	assert.True(t, MkcertAvailable())

	t.Setenv("SHOPWARE_CLI_PROXY_DISABLE_MKCERT", "1")
	assert.False(t, MkcertAvailable())
}

func TestMkcertCAPath(t *testing.T) {
	caroot := t.TempDir()
	installFakeMkcert(t, caroot)

	caPath, err := MkcertCAPath(t.Context())
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(caroot, "rootCA.pem"), caPath)
}

func TestEnsureCertificateUsesMkcert(t *testing.T) {
	caroot := t.TempDir()
	argsFile := installFakeMkcert(t, caroot)

	dir := t.TempDir()

	info, err := EnsureCertificate(t.Context(), dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)
	assert.True(t, info.Mkcert)
	assert.True(t, info.Changed)
	assert.Equal(t, filepath.Join(caroot, "rootCA.pem"), info.CAPath)

	args, err := os.ReadFile(argsFile)
	assert.NoError(t, err)
	assert.Contains(t, string(args), "-cert-file "+ServerCertPath(dir))
	assert.Contains(t, string(args), "-key-file "+ServerKeyPath(dir))
	assert.Contains(t, string(args), DefaultDomain+" *."+DefaultDomain)
	assert.FileExists(t, ServerCertPath(dir))
}

func TestEnsureCertificateReusesValidMkcertCert(t *testing.T) {
	// Build a valid CA + server certificate with our own generator, then
	// present the CA as the mkcert root: EnsureCertificate must not regenerate.
	source := t.TempDir()
	_, err := EnsureCA(source)
	assert.NoError(t, err)
	_, err = EnsureServerCert(source, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)

	caroot := t.TempDir()
	caPem, err := os.ReadFile(CACertPath(source))
	assert.NoError(t, err)
	assert.NoError(t, os.WriteFile(filepath.Join(caroot, "rootCA.pem"), caPem, 0o644))

	argsFile := installFakeMkcert(t, caroot)

	info, err := EnsureCertificate(t.Context(), source, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)
	assert.True(t, info.Mkcert)
	assert.False(t, info.Changed, "existing certificate issued by the mkcert CA must be reused")
	assert.NoFileExists(t, argsFile, "mkcert must not be called for generation")
}

func TestEnsureCertificateFallsBackToOwnCA(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	dir := t.TempDir()

	info, err := EnsureCertificate(t.Context(), dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)
	assert.False(t, info.Mkcert)
	assert.True(t, info.Changed)
	assert.Equal(t, CACertPath(dir), info.CAPath)
	assert.FileExists(t, ServerCertPath(dir))
}

func TestCertIssuedBy(t *testing.T) {
	dir := t.TempDir()
	_, err := EnsureCA(dir)
	assert.NoError(t, err)
	_, err = EnsureServerCert(dir, CertHosts(DefaultDomain, nil))
	assert.NoError(t, err)

	assert.True(t, certIssuedBy(ServerCertPath(dir), CACertPath(dir)))

	other := t.TempDir()
	_, err = EnsureCA(other)
	assert.NoError(t, err)

	assert.False(t, certIssuedBy(ServerCertPath(dir), CACertPath(other)))
}

func TestMkcertArgsQuoting(t *testing.T) {
	// Sanity check that the fake script's argument recording keeps host order.
	caroot := t.TempDir()
	argsFile := installFakeMkcert(t, caroot)

	dir := t.TempDir()

	assert.NoError(t, generateWithMkcert(t.Context(), dir, []string{"a.example.test", "b.example.test"}))

	args, err := os.ReadFile(argsFile)
	assert.NoError(t, err)
	assert.True(t, strings.HasSuffix(strings.TrimSpace(string(args)), "a.example.test b.example.test"))
}

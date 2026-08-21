package proxy

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/mkcert"
)

func TestContainerCABundlePathDeterministic(t *testing.T) {
	withTempStateDir(t)

	a, err := ContainerCABundlePath("ghcr.io/shopware/docker-dev:php8.3-node24-caddy")
	require.NoError(t, err)

	again, err := ContainerCABundlePath("ghcr.io/shopware/docker-dev:php8.3-node24-caddy")
	require.NoError(t, err)
	assert.Equal(t, a, again, "same image must map to the same path")

	other, err := ContainerCABundlePath("ghcr.io/shopware/docker-dev:php8.2-node24-caddy")
	require.NoError(t, err)
	assert.NotEqual(t, a, other, "different images must map to different paths")

	dir, err := StateDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, caBundleDirName), filepath.Dir(a))
	assert.True(t, strings.HasSuffix(a, ".crt"))
}

func TestCombineCABundle(t *testing.T) {
	t.Parallel()

	// The two PEM blocks are joined by a newline, so they never run together even
	// if the system bundle has no trailing newline.
	assert.Equal(t, "SYSTEM\nPROXYCA", string(combineCABundle([]byte("SYSTEM"), []byte("PROXYCA"))))
}

func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bundle.crt")

	require.NoError(t, writeFileAtomic(path, []byte("data"), 0o644))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "data", string(got))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())

	// The temp file is renamed into place, leaving nothing behind.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// TestEnsureContainerCABundleIntegration builds the combined bundle from a real
// image and checks that a certificate signed by the proxy CA verifies against
// it while the image's public CAs are preserved — i.e. a shop's HTTPS self-call
// to its own APP_URL would be trusted, which was the whole point of the mount.
// Docker is used only to read the image's system bundle; verification is pure Go
// (no openssl needed in the image). Skipped when Docker is unavailable.
func TestEnsureContainerCABundleIntegration(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	ctx := t.Context()
	if _, err := runDocker(ctx, "version", "--format", "{{.Server.Version}}"); err != nil {
		t.Skip("docker daemon not running")
	}

	// Use the real shop image (it ships the system trust store the fix relies
	// on). It is large, so skip rather than pull when it is not already local.
	const image = "ghcr.io/shopware/docker-dev:php8.3-node24-caddy"
	if _, err := runDocker(ctx, "image", "inspect", image); err != nil {
		t.Skipf("%s not present locally", image)
	}

	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("CAROOT", t.TempDir())

	caPath, err := CACertPath()
	require.NoError(t, err)

	bundlePath, err := EnsureContainerCABundle(ctx, image, caPath)
	require.NoError(t, err)
	require.FileExists(t, bundlePath)

	bundlePEM, err := os.ReadFile(bundlePath)
	require.NoError(t, err)

	// The combined bundle carries more certificates than the image's system store
	// alone (the appended proxy CA).
	systemPEM, err := imageSystemCABundle(ctx, image)
	require.NoError(t, err)
	assert.Greater(t,
		strings.Count(string(bundlePEM), "BEGIN CERTIFICATE"),
		strings.Count(string(systemPEM), "BEGIN CERTIFICATE"),
		"bundle should contain the public CAs plus the proxy CA")

	// A leaf signed by the proxy CA verifies against the combined bundle: the
	// public roots are still there and our CA was added.
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(bundlePEM), "bundle must be valid PEM")

	dir := t.TempDir()
	leafPath := filepath.Join(dir, "leaf.pem")
	keyPath := filepath.Join(dir, "leaf-key.pem")

	ca, _, err := mkcert.LoadOrCreateCA()
	require.NoError(t, err)
	require.NoError(t, ca.MakeCert([]string{"myshop.shopware.local"}, leafPath, keyPath))

	leaf := readLeafCertificate(t, leafPath)
	_, err = leaf.Verify(x509.VerifyOptions{Roots: pool, DNSName: "myshop.shopware.local"})
	assert.NoError(t, err, "leaf signed by the proxy CA must verify against the combined bundle")
}

func readLeafCertificate(t *testing.T, path string) *x509.Certificate {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	block, _ := pem.Decode(content)
	require.NotNil(t, block)

	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)

	return cert
}

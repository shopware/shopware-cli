package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var overrideOpts = &ProxyOptions{
	Hostname:    "my-shop.shopware.local",
	NetworkName: "shopware-cli-proxy",
	CAPath:      "/state/mkcert/rootCA.pem",
}

func TestGenerateComposeOverride(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "shopware/core", Version: "6.6.0.0"},
			{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
			{Name: "shopware/elasticsearch", Version: "6.6.0.0"},
		},
	}

	result, err := GenerateComposeOverride(lock, overrideOpts, []string{"worker", "scheduler"})
	require.NoError(t, err)
	override := string(result)

	// Marked as CLI-managed so up/down never touch user files.
	assert.True(t, strings.HasPrefix(override, overrideMarker))

	// The base file's fixed host ports are cleared, not merged into.
	assert.Contains(t, override, "ports: !reset []")

	// Every routed service is labeled for Traefik and joins the shared network.
	assert.Contains(t, override, "Host(`my-shop.shopware.local`)")
	assert.Contains(t, override, "Host(`admin-watch.my-shop.shopware.local`)")
	assert.Contains(t, override, "Host(`storefront-watch.my-shop.shopware.local`)")
	assert.Contains(t, override, "Host(`adminer.my-shop.shopware.local`)")
	assert.Contains(t, override, "Host(`mailer.my-shop.shopware.local`)")
	assert.Contains(t, override, "Host(`lavinmq.my-shop.shopware.local`)")
	assert.Contains(t, override, "Host(`opensearch.my-shop.shopware.local`)")
	assert.Contains(t, override, "websecure")
	assert.Contains(t, override, "external: true")

	// TLS terminates at Traefik, so the web container must trust it.
	assert.Contains(t, override, "TRUSTED_PROXIES")

	// The CA is mounted into the web container and Node points at it, so the
	// shop can call its own APP_URL over HTTPS.
	assert.Contains(t, override, "/state/mkcert/rootCA.pem:/usr/local/share/ca-certificates/shopware-cli-proxy.crt:ro")
	assert.Contains(t, override, "NODE_EXTRA_CA_CERTS")

	// worker/scheduler join the shared network + carry the CA so their PHP
	// console processes can call APP_URL, but they get no Traefik route.
	assert.Contains(t, override, "worker:")
	assert.Contains(t, override, "scheduler:")
	assert.NotContains(t, override, "Host(`worker")
	assert.NotContains(t, override, "Host(`scheduler")

	// The override only modifies services, it must not define images.
	assert.NotContains(t, override, "image:")
}

func TestGenerateComposeOverrideWithoutCA(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}

	result, err := GenerateComposeOverride(lock, &ProxyOptions{Hostname: "my-shop.shopware.local", NetworkName: "shopware-cli-proxy"}, nil)
	require.NoError(t, err)

	assert.NotContains(t, string(result), "NODE_EXTRA_CA_CERTS")
	assert.NotContains(t, string(result), "ca-certificates")
}

func TestGenerateComposeOverrideBackgroundServicesAreNetworkOnly(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}

	// Without background services, worker/scheduler must not appear.
	none, err := GenerateComposeOverride(lock, overrideOpts, nil)
	require.NoError(t, err)
	assert.NotContains(t, string(none), "worker:")
	assert.NotContains(t, string(none), "scheduler:")

	// With them, they join the shared network and mount the CA but expose no
	// port and no Traefik router.
	with, err := GenerateComposeOverride(lock, overrideOpts, []string{"worker"})
	require.NoError(t, err)
	assert.Contains(t, string(with), "worker:")
	assert.Contains(t, string(with), "shopware-cli-proxy")
	assert.NotContains(t, string(with), "Host(`worker")
}

func TestGenerateComposeOverrideSkipsAbsentServices(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "shopware/core", Version: "6.6.0.0"},
		},
	}

	result, err := GenerateComposeOverride(lock, overrideOpts, nil)
	require.NoError(t, err)

	assert.NotContains(t, string(result), "lavinmq")
	assert.NotContains(t, string(result), "opensearch")
}

func TestWriteAndRemoveComposeOverride(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestComposerLock(t, dir)

	require.NoError(t, WriteComposeOverride(dir, overrideOpts))
	path := filepath.Join(dir, "compose.override.yaml")
	assert.FileExists(t, path)

	// Writing again over our own file is fine.
	require.NoError(t, WriteComposeOverride(dir, overrideOpts))

	require.NoError(t, RemoveComposeOverride(dir))
	assert.NoFileExists(t, path)

	// Removing when nothing exists is fine too.
	require.NoError(t, RemoveComposeOverride(dir))
}

func TestComposeOverrideRefusesUserFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeTestComposerLock(t, dir)

	path := filepath.Join(dir, "compose.override.yaml")
	require.NoError(t, os.WriteFile(path, []byte("services:\n  web:\n    environment:\n      FOO: bar\n"), 0o644))

	assert.Error(t, WriteComposeOverride(dir, overrideOpts))
	assert.Error(t, RemoveComposeOverride(dir))

	// The user's file is untouched.
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(content), "FOO: bar")
}

func writeTestComposerLock(t *testing.T, dir string) {
	t.Helper()

	lock := `{"packages": [{"name": "shopware/core", "version": "6.6.0.0"}], "packages-dev": []}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644))
}

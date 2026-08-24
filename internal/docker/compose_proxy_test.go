package docker

import (
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func proxyComposeOptions() *ComposeOptions {
	return &ComposeOptions{
		Proxy: &ProxyOptions{
			Hostname:       "my-shop.shopware.local",
			NetworkName:    "shopware-cli-proxy",
			CABundlePath:   "/state/proxy/ca-bundles/abcd1234.crt",
			AdminWatchPort: 5173,
		},
	}
}

func TestGenerateComposeFileProxyMode(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "shopware/core", Version: "6.6.0.0"},
			{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
			{Name: "shopware/elasticsearch", Version: "6.6.0.0"},
		},
	}

	opts := proxyComposeOptions()
	opts.DedicatedWorker = true // exercise the worker/scheduler services

	result, err := GenerateComposeFile(lock, opts)
	require.NoError(t, err)
	out := string(result)

	// Every routed service is labeled for Traefik and joins the shared network.
	assert.Contains(t, out, "Host(`my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`admin-watch.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`storefront-watch.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`adminer.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`mailer.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`lavinmq.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`opensearch.my-shop.shopware.local`)")
	assert.Contains(t, out, "websecure")
	assert.Contains(t, out, "external: true")

	// The routed services publish no fixed host ports.
	assert.NotContains(t, out, "8000:8000")
	assert.NotContains(t, out, "9080:8080")
	assert.NotContains(t, out, "8025:8025")

	// The database keeps its random loopback port so host tools (and the sales
	// channel URL repoint) can still reach it in proxy mode.
	assert.Contains(t, out, "127.0.0.1::3306")

	// The deprecated storefront watcher gets a second router under the same
	// hostname on the dedicated asset entrypoint (asset + HMR server).
	assert.Contains(t, out, "sfassets")
	assert.Contains(t, out, "storefront-watch-assets")

	// TLS terminates at Traefik, so the web container must trust its headers.
	assert.Contains(t, out, "127.0.0.1,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16")

	// APP_URL is NOT pinned as a container env var; proxy up writes it into
	// .env.local before the container starts, keeping the file authoritative.
	assert.NotContains(t, out, "APP_URL:")

	// The combined CA bundle is mounted over the system trust store and Node
	// points at it, so PHP/curl/Node trust the proxy when the shop calls its own
	// APP_URL over HTTPS.
	assert.Contains(t, out, "/state/proxy/ca-bundles/abcd1234.crt:/etc/ssl/certs/ca-certificates.crt:ro")
	assert.Contains(t, out, "NODE_EXTRA_CA_CERTS")

	// worker/scheduler join the shared network + carry the CA (via the shared
	// web environment) but get no Traefik route.
	assert.Contains(t, out, "worker:")
	assert.Contains(t, out, "scheduler:")
	assert.NotContains(t, out, "Host(`worker")
	assert.NotContains(t, out, "Host(`scheduler")
}

func TestGenerateComposeFileProxyAdminWatchPort(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.10.2"}}}

	opts := proxyComposeOptions()
	opts.Proxy.AdminWatchPort = 8080
	webpack, err := GenerateComposeFile(lock, opts)
	require.NoError(t, err)
	// Tie the port to the admin-watch router specifically (adminer is also 8080).
	assert.Contains(t, string(webpack), "-admin-watch.loadbalancer.server.port: \"8080\"")

	opts.Proxy.AdminWatchPort = 5173
	vite, err := GenerateComposeFile(lock, opts)
	require.NoError(t, err)
	assert.Contains(t, string(vite), "-admin-watch.loadbalancer.server.port: \"5173\"")
}

func TestGenerateComposeFileProxyWithoutCA(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}

	opts := &ComposeOptions{Proxy: &ProxyOptions{Hostname: "my-shop.shopware.local", NetworkName: "shopware-cli-proxy"}}
	result, err := GenerateComposeFile(lock, opts)
	require.NoError(t, err)

	assert.NotContains(t, string(result), "NODE_EXTRA_CA_CERTS")
	assert.NotContains(t, string(result), "ca-certificates")
}

func TestGenerateComposeFileProxySkipsAbsentServices(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}

	result, err := GenerateComposeFile(lock, proxyComposeOptions())
	require.NoError(t, err)

	assert.NotContains(t, string(result), "lavinmq")
	assert.NotContains(t, string(result), "opensearch")
	assert.NotContains(t, string(result), "redis")
	assert.NotContains(t, string(result), "rustfs")
}

func TestGenerateComposeFileProxyKeepsRustfsHostPorts(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "shopware/core", Version: "6.6.0.0"},
			{Name: "shopware/k8s-meta", Version: "1.0.0"},
			{Name: "symfony/redis-messenger", Version: "v7.0.0"},
		},
	}

	result, err := GenerateComposeFile(lock, proxyComposeOptions())
	require.NoError(t, err)
	out := string(result)

	// PUBLIC_URL is baked into env, so S3 and the console stay on fixed host
	// ports even when the rest of the stack is proxied.
	assert.Contains(t, out, "9000:9000")
	assert.Contains(t, out, "9001:9001")
	assert.Contains(t, out, "127.0.0.1::6379")
	assert.Contains(t, out, "K8S_FILESYSTEM_PUBLIC_URL: http://127.0.0.1:9000/shopware-public")
	assert.NotContains(t, out, "Host(`rustfs.")
}

func TestGenerateComposeFilePlainModeHasPorts(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}

	result, err := GenerateComposeFile(lock, &ComposeOptions{})
	require.NoError(t, err)
	out := string(result)

	// Plain mode keeps fixed host ports and adds no proxy routing.
	assert.Contains(t, out, "8000:8000")
	assert.NotContains(t, out, "traefik.enable")
	assert.NotContains(t, out, "external: true")
	assert.NotContains(t, out, "Host(`")
}

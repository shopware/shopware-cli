package docker

import (
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func proxyComposeOptions() Environment {
	return Environment{
		proxy: &Proxy{
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
	opts.dedicatedWorker = true // exercise the worker/scheduler services

	result, err := envFor(lock, opts).composeYAML()
	require.NoError(t, err)
	out := string(result)

	// Every routed service is labeled for Traefik and joins the shared network.
	assert.Contains(t, out, "Host(`my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`admin-watch.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`storefront-watch.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`adminer.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`mailer.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`queue.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`search.my-shop.shopware.local`)")
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
	opts.proxy.AdminWatchPort = 8080
	webpack, err := envFor(lock, opts).composeYAML()
	require.NoError(t, err)
	// Tie the port to the admin-watch router specifically (adminer is also 8080).
	assert.Contains(t, string(webpack), "-admin-watch.loadbalancer.server.port: \"8080\"")

	opts.proxy.AdminWatchPort = 5173
	vite, err := envFor(lock, opts).composeYAML()
	require.NoError(t, err)
	assert.Contains(t, string(vite), "-admin-watch.loadbalancer.server.port: \"5173\"")
}

func TestGenerateComposeFileProxyWithoutCA(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}

	opts := Environment{proxy: &Proxy{Hostname: "my-shop.shopware.local", NetworkName: "shopware-cli-proxy"}}
	result, err := envFor(lock, opts).composeYAML()
	require.NoError(t, err)

	assert.NotContains(t, string(result), "NODE_EXTRA_CA_CERTS")
	assert.NotContains(t, string(result), "ca-certificates")
}

func TestGenerateComposeFileProxySkipsAbsentServices(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}

	result, err := envFor(lock, proxyComposeOptions()).composeYAML()
	require.NoError(t, err)

	assert.NotContains(t, string(result), "lavinmq")
	assert.NotContains(t, string(result), "opensearch")
	assert.NotContains(t, string(result), "redis")
	assert.NotContains(t, string(result), "rustfs")
}

func TestGenerateComposeFileProxyRoutesRustfs(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "shopware/core", Version: "6.6.0.0"},
			{Name: "shopware/k8s-meta", Version: "1.0.0"},
			{Name: "symfony/redis-messenger", Version: "v7.0.0"},
		},
	}

	result, err := envFor(lock, proxyComposeOptions()).composeYAML()
	require.NoError(t, err)
	out := string(result)

	// S3 API and console are local-domain routes so media URLs stay HTTPS.
	assert.Contains(t, out, "Host(`s3.my-shop.shopware.local`)")
	assert.Contains(t, out, "Host(`storage.my-shop.shopware.local`)")
	assert.Contains(t, out, "K8S_FILESYSTEM_PUBLIC_URL: https://s3.my-shop.shopware.local/shopware-public")
	assert.Contains(t, out, "K8S_FILESYSTEM_ENDPOINT: http://my-shop-shopware-local-storage:9000")
	assert.Contains(t, out, "127.0.0.1::3306")
	assert.NotContains(t, out, "127.0.0.1::6379")
	assert.NotContains(t, out, "9000:9000")
	assert.NotContains(t, out, "9001:9001")
	assert.NotContains(t, out, "http://127.0.0.1:9000/shopware-public")
}

func TestGenerateComposeFilePlainModeHasPorts(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}

	result, err := envFor(lock, Environment{}).composeYAML()
	require.NoError(t, err)
	out := string(result)

	// Plain mode keeps fixed host ports and adds no proxy routing.
	assert.Contains(t, out, "8000:8000")
	assert.NotContains(t, out, "traefik.enable")
	assert.NotContains(t, out, "external: true")
	assert.NotContains(t, out, "Host(`")
}

// TestGenerateComposeFileProxyInternalAliases guards issue #1484: on the shared
// proxy network every project's services would otherwise advertise the same
// bare name (search, mailer, ...), so an internal call from one project
// could reach another's container. In proxy mode the services must join the
// shared network under a project-unique alias, and web/console must address
// them by that alias.
func TestGenerateComposeFileProxyInternalAliases(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "shopware/core", Version: "6.6.0.0"},
			{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
			{Name: "shopware/elasticsearch", Version: "6.6.0.0"},
		},
	}

	out, err := envFor(lock, proxyComposeOptions()).composeYAML()
	require.NoError(t, err)
	compose := string(out)

	// Internal traffic uses the project-unique alias, not the bare service name.
	assert.Contains(t, compose, "MAILER_DSN: smtp://my-shop-shopware-local-mailer:1025")
	assert.Contains(t, compose, "OPENSEARCH_URL: http://my-shop-shopware-local-search:9200")
	assert.Contains(t, compose, "MESSENGER_TRANSPORT_DSN: amqp://guest:guest@my-shop-shopware-local-queue:5672")

	// The bare, collision-prone forms must be gone in proxy mode.
	assert.NotContains(t, compose, "http://search:9200")
	assert.NotContains(t, compose, "smtp://mailer:1025")
	assert.NotContains(t, compose, "amqp://guest:guest@queue:5672")

	// Each proxied service advertises the project-unique alias on the shared
	// network, so parallel projects never collide on the bare name there.
	assert.Contains(t, compose, "- my-shop-shopware-local-search")
	assert.Contains(t, compose, "- my-shop-shopware-local-mailer")
	assert.Contains(t, compose, "- my-shop-shopware-local-queue")

	// The console processes join the shared network without an alias: nothing
	// addresses them by name.
	assert.NotContains(t, compose, "- my-shop-shopware-local-worker")

	// The database is not on the shared network (it publishes a loopback port),
	// so it keeps the bare host and never collided.
	assert.Contains(t, compose, "DATABASE_URL: mysql://root:root@database/shopware")
}

// TestGenerateComposeFilePlainModeKeepsBareServiceHosts is the regression guard
// for the alias change: without proxy mode, services are addressed by their
// bare names (no shared network, no collision to avoid).
func TestGenerateComposeFilePlainModeKeepsBareServiceHosts(t *testing.T) {
	t.Parallel()

	lock := &composer.Lock{
		Packages: []composer.LockPackage{
			{Name: "shopware/core", Version: "6.6.0.0"},
			{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
			{Name: "shopware/elasticsearch", Version: "6.6.0.0"},
		},
	}

	out, err := envFor(lock, Environment{}).composeYAML()
	require.NoError(t, err)
	compose := string(out)

	assert.Contains(t, compose, "MAILER_DSN: smtp://mailer:1025")
	assert.Contains(t, compose, "OPENSEARCH_URL: http://search:9200")
	assert.Contains(t, compose, "MESSENGER_TRANSPORT_DSN: amqp://guest:guest@queue:5672")
	assert.NotContains(t, compose, "my-shop-shopware-local-")
}

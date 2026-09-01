package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestServicesCatalogInvariants(t *testing.T) {
	t.Parallel()

	keys := map[string]string{}
	subdomains := map[string]string{}
	defaults := map[int]string{}

	for _, svc := range Services {
		require.NotEmpty(t, svc.Name)
		require.NotEmpty(t, svc.Label)

		uiCount := 0
		for _, ep := range svc.Endpoints {
			require.NotZero(t, ep.ContainerPort, "%s: endpoint %q needs a container port", svc.Name, ep.Label)

			if ep.Key != "" {
				if prev, dup := keys[ep.Key]; dup {
					t.Fatalf("docker.ports key %q used by both %s and %s", ep.Key, prev, svc.Name)
				}
				keys[ep.Key] = svc.Name

				require.NotEmpty(t, ep.Label, "%s.%s: keyed endpoint needs a conflict-message label", svc.Name, ep.Key)
				require.NotZero(t, ep.DefaultHostPort, "%s.%s: keyed endpoint needs a default host port", svc.Name, ep.Key)
				if prev, dup := defaults[ep.DefaultHostPort]; dup {
					t.Fatalf("default host port %d used by both %s and %s", ep.DefaultHostPort, prev, ep.Key)
				}
				defaults[ep.DefaultHostPort] = ep.Key
			}

			if ep.Subdomain != "" {
				if prev, dup := subdomains[ep.Subdomain]; dup {
					t.Fatalf("proxy subdomain %q used by both %s and %s", ep.Subdomain, prev, svc.Name)
				}
				subdomains[ep.Subdomain] = svc.Name
			}

			if ep.Role == RoleUI {
				uiCount++
			}
		}

		if !svc.Hidden {
			require.Equal(t, 1, uiCount, "%s: visible services need exactly one UI endpoint", svc.Name)
		} else {
			require.LessOrEqual(t, uiCount, 1, "%s: at most one UI endpoint", svc.Name)
		}
	}
}

func TestActiveServices(t *testing.T) {
	t.Parallel()

	names := func(services []ServiceDefinition) []string {
		out := make([]string, 0, len(services))
		for _, svc := range services {
			out = append(out, svc.Name)
		}
		return out
	}

	base := names(ActiveServices(LockFeatures{}))
	assert.Contains(t, base, "web")
	assert.Contains(t, base, "database")
	assert.Contains(t, base, "adminer")
	assert.Contains(t, base, "mailer")
	assert.NotContains(t, base, "lavinmq")
	assert.NotContains(t, base, "opensearch")
	assert.NotContains(t, base, "redis")
	assert.NotContains(t, base, "rustfs")
	assert.NotContains(t, base, "rustfs-init")

	full := names(ActiveServices(LockFeatures{AMQP: true, Elasticsearch: true, RedisMessenger: true, S3: true}))
	assert.Contains(t, full, "lavinmq")
	assert.Contains(t, full, "opensearch")
	assert.Contains(t, full, "redis")
	assert.Contains(t, full, "rustfs")
	assert.Contains(t, full, "rustfs-init")

	// Redis follows NeedsRedis: redis messenger or S3.
	assert.Contains(t, names(ActiveServices(LockFeatures{S3: true})), "redis")
	assert.Contains(t, names(ActiveServices(LockFeatures{RedisMessenger: true})), "redis")
}

func TestRoutedSubdomains(t *testing.T) {
	t.Parallel()

	assert.Equal(t,
		[]string{"", SubdomainAdminWatch, SubdomainStorefrontWatch, "adminer", "mailer"},
		RoutedSubdomains(LockFeatures{}))
	assert.Equal(t,
		[]string{"", SubdomainAdminWatch, SubdomainStorefrontWatch, "adminer", "mailer", "lavinmq", "opensearch", "s3", "rustfs"},
		RoutedSubdomains(LockFeatures{AMQP: true, Elasticsearch: true, S3: true}))
}

func TestServiceByName(t *testing.T) {
	t.Parallel()

	assert.Nil(t, ServiceByName("rabbitmq"), "user override services are not generated")

	mailer := ServiceByName("mailer")
	require.NotNil(t, mailer)
	assert.Equal(t, "Mailpit", mailer.Label)
}

func TestEndpointByKey(t *testing.T) {
	t.Parallel()

	assert.Nil(t, EndpointByKey("unknown"))

	ep := EndpointByKey(shop.DockerPortMailerWeb)
	require.NotNil(t, ep)
	assert.Equal(t, 8025, ep.ContainerPort)
	assert.Equal(t, 8025, ep.DefaultHostPort)
	assert.Equal(t, "mailer", ep.Subdomain)
}

func TestShopEndpoint(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 8000, ShopEndpoint().ContainerPort)
	assert.Equal(t, 8000, ShopEndpoint().DefaultHostPort)
}

func TestEndpointURL(t *testing.T) {
	t.Parallel()

	adminer := EndpointByKey(shop.DockerPortAdminer)
	require.NotNil(t, adminer)

	// Plain mode: default, override, disabled.
	assert.Equal(t, "http://127.0.0.1:9080", EndpointURL(*adminer, "", nil))
	assert.Equal(t, "http://127.0.0.1:9999", EndpointURL(*adminer, "", shop.ConfigDockerPorts{shop.DockerPortAdminer: 9999}))
	assert.Empty(t, EndpointURL(*adminer, "", shop.ConfigDockerPorts{shop.DockerPortAdminer: shop.DockerPortDisabled}))

	// Proxy mode: routed at the endpoint's subdomain.
	assert.Equal(t, "https://adminer.my-shop.local", EndpointURL(*adminer, "my-shop.local", nil))

	// An unrouted endpoint has no proxy URL; an endpoint without a config key
	// is never published in plain mode.
	smtp := EndpointByKey(shop.DockerPortMailerSMTP)
	require.NotNil(t, smtp)
	assert.Empty(t, EndpointURL(*smtp, "my-shop.local", nil))
	assert.Empty(t, EndpointURL(Endpoint{ContainerPort: 3306}, "", nil))
}

func TestServiceProxyURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://adminer.my-shop.local", ServiceByName("adminer").ProxyURL("my-shop.local"))
	// The rustfs UI endpoint is the console, routed at its own subdomain.
	assert.Equal(t, "https://rustfs.my-shop.local", ServiceByName("rustfs").ProxyURL("my-shop.local"))
	// Web's UI endpoint has no subdomain (its proxy routes are custom).
	assert.Empty(t, ServiceByName("web").ProxyURL("my-shop.local"))
}

func TestServiceAccessURL(t *testing.T) {
	t.Parallel()

	rustfs := ServiceByName("rustfs")
	require.NotNil(t, rustfs)

	// Plain mode: the published port of the UI endpoint (the console, 9001)
	// wins, even when other endpoints are published too.
	assert.Equal(t, "http://127.0.0.1:19001", rustfs.AccessURL(map[int]int{9000: 19000, 9001: 19001}, ""))
	// Proxy mode: the explicit console subdomain, not the service name.
	assert.Equal(t, "https://rustfs.my-shop.local", rustfs.AccessURL(nil, "my-shop.local"))
	// Neither published nor proxied: unreachable.
	assert.Empty(t, rustfs.AccessURL(nil, ""))

	// Services without a UI endpoint never get a URL.
	redis := ServiceByName("redis")
	require.NotNil(t, redis)
	assert.Empty(t, redis.AccessURL(map[int]int{6379: 16379}, ""))
}

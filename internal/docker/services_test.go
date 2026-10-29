package docker

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutedSubdomains(t *testing.T) {
	t.Parallel()

	base := &Environment{}
	assert.Equal(t,
		[]string{"", subdomainAdminWatch, subdomainStorefrontWatch, "adminer", "mailer"},
		base.RoutedSubdomains())

	full := &Environment{features: features{AMQP: true, Elasticsearch: true, S3: true}}
	assert.Equal(t,
		[]string{"", subdomainAdminWatch, subdomainStorefrontWatch, "adminer", "mailer", "queue", "search", "s3", "storage"},
		full.RoutedSubdomains())

	vite := &Environment{storefrontUsesVite: true}
	assert.Equal(t,
		[]string{"", subdomainAdminWatch, subdomainStorefrontWatch, "adminer", "mailer"},
		vite.RoutedSubdomains())
}

func TestServiceSubdomains(t *testing.T) {
	t.Parallel()

	webService := byName(ServiceWeb)
	require.NotNil(t, webService)

	// The web hostnames are stable across legacy and Vite watcher modes.
	assert.Equal(t, []string{"", subdomainAdminWatch, subdomainStorefrontWatch}, serviceSubdomains(*webService, &Environment{}))
	assert.Equal(t, []string{"", subdomainAdminWatch, subdomainStorefrontWatch}, serviceSubdomains(*webService, &Environment{storefrontUsesVite: true}))
	// Endpoint-derived: only endpoints with a subdomain are routed.
	assert.Equal(t, []string{"mailer"}, serviceSubdomains(*byName(ServiceMailer), &Environment{}))
	assert.Equal(t, []string{"s3", "storage"}, serviceSubdomains(*byName(ServiceStorage), &Environment{}))
	assert.Empty(t, serviceSubdomains(*byName(ServiceDatabase), &Environment{}))
}

func TestRoutes(t *testing.T) {
	t.Parallel()

	plain := &Environment{}
	assert.Empty(t, plain.routes(*byName(ServiceMailer)), "nothing is routed in fixed-port mode")

	proxied := &Environment{proxy: &Proxy{Hostname: "my-shop.shopware.local"}}
	assert.Len(t, proxied.routes(*byName(ServiceMailer)), 1)
	assert.Empty(t, proxied.routes(*byName(ServiceDatabase)), "the database has no proxy route in either mode")
}

func TestPortBindings(t *testing.T) {
	t.Parallel()

	mailer := *byName(ServiceMailer)
	assert.Equal(t, []string{"1025:1025", "8025:8025"}, portBindings(mailer, nil))
	assert.Equal(t, []string{"1025:1025", "18025:8025"}, portBindings(mailer, Ports{PortHTTP: 18025}))
	assert.Equal(t, []string{"8025:8025"}, portBindings(mailer, Ports{PortSMTP: PortDisabled}))

	// Nameless endpoints are never published by the generator.
	assert.Empty(t, portBindings(*byName(ServiceCache), nil))

	// A loopback endpoint without a default publishes on a random loopback
	// port until pinned.
	database := *byName(ServiceDatabase)
	assert.Equal(t, []string{"127.0.0.1::3306"}, portBindings(database, nil))
	assert.Equal(t, []string{"127.0.0.1:3307:3306"}, portBindings(database, Ports{PortMySQL: 3307}))
	assert.Empty(t, portBindings(database, Ports{PortMySQL: PortDisabled}))

	// The queue broker and the search node are published on loopback only, so
	// they are reachable from the host but not from the network.
	assert.Equal(t, []string{"127.0.0.1:15672:15672", "127.0.0.1:5672:5672"}, portBindings(*byName(ServiceQueue), nil))
	assert.Equal(t, []string{"127.0.0.1:19200:9200"}, portBindings(*byName(ServiceSearch), Ports{PortHTTP: 19200}))
}

func TestStorefrontViteEndpointDoesNotMutateCatalog(t *testing.T) {
	t.Parallel()

	webCatalogService := byName(ServiceWeb)
	require.NotNil(t, webCatalogService)
	catalogEndpoints := slices.Clone(webCatalogService.Endpoints)

	vite := &Environment{storefrontUsesVite: true}
	services := vite.activeServices()
	web := serviceByName(services, ServiceWeb)
	require.Equal(t, ServiceWeb, web.Name)
	assert.Equal(
		t,
		endpointByName(catalogEndpoints, PortStorefrontVite),
		endpointByName(web.Endpoints, PortStorefrontVite))
	assert.Empty(t, endpointByName(web.Endpoints, PortStorefrontWatcher))
	assert.Empty(t, endpointByName(web.Endpoints, PortStorefrontWatcherAssets))

	assert.Equal(t, catalogEndpoints, webCatalogService.Endpoints)
}

func serviceByName(services []service, name string) service {
	for _, svc := range services {
		if svc.Name == name {
			return svc
		}
	}
	return service{}
}

func endpointByName(endpoints []endpoint, name string) endpoint {
	for _, ep := range endpoints {
		if ep.Name == name {
			return ep
		}
	}
	return endpoint{}
}

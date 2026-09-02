package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogInvariants(t *testing.T) {
	t.Parallel()

	names := map[string]struct{}{}
	subdomains := map[string]string{}
	defaults := map[int]string{}

	for _, svc := range catalog {
		require.NotEmpty(t, svc.Name)
		require.NotEmpty(t, svc.Label)
		require.NotNil(t, svc.build, "%s: every service needs a build function", svc.Name)
		if _, dup := names[svc.Name]; dup {
			t.Fatalf("service %q listed twice", svc.Name)
		}
		names[svc.Name] = struct{}{}

		endpointNames := map[string]struct{}{}
		uiCount := 0
		for _, ep := range svc.Endpoints {
			require.NotZero(t, ep.ContainerPort, "%s: endpoint %q needs a container port", svc.Name, ep.Label)

			if ep.Name != "" {
				if _, dup := endpointNames[ep.Name]; dup {
					t.Fatalf("%s: ports key %q used twice", svc.Name, ep.Name)
				}
				endpointNames[ep.Name] = struct{}{}
				require.NotEmpty(t, ep.Label, "%s.%s: published endpoint needs a conflict-message label", svc.Name, ep.Name)

				if ep.DefaultHostPort != 0 {
					if prev, dup := defaults[ep.DefaultHostPort]; dup {
						t.Fatalf("default host port %d used by both %s and %s.%s", ep.DefaultHostPort, prev, svc.Name, ep.Name)
					}
					defaults[ep.DefaultHostPort] = svc.Name + "." + ep.Name
				}
			} else {
				require.Zero(t, ep.DefaultHostPort, "%s: an endpoint without a ports key cannot have a default host port", svc.Name)
			}

			if ep.Subdomain != "" {
				if prev, dup := subdomains[ep.Subdomain]; dup {
					t.Fatalf("proxy subdomain %q used by both %s and %s", ep.Subdomain, prev, svc.Name)
				}
				subdomains[ep.Subdomain] = svc.Name
			}

			if ep.Role == roleUI {
				uiCount++
			}
		}

		if !svc.Hidden {
			require.Equal(t, 1, uiCount, "%s: visible services need exactly one UI endpoint", svc.Name)
		} else {
			require.LessOrEqual(t, uiCount, 1, "%s: at most one UI endpoint", svc.Name)
		}

		variantNames := map[string]struct{}{}
		for _, v := range svc.Variants {
			require.NotEmpty(t, v.Name, "%s: variants need a name", svc.Name)
			require.NotEmpty(t, v.Image, "%s.%s: variants need an image", svc.Name, v.Name)
			require.NotEmpty(t, v.DefaultTag, "%s.%s: variants need a default tag", svc.Name, v.Name)
			if _, dup := variantNames[v.Name]; dup {
				t.Fatalf("%s: variant %q listed twice", svc.Name, v.Name)
			}
			variantNames[v.Name] = struct{}{}
		}
	}
}

func TestActiveServices(t *testing.T) {
	t.Parallel()

	names := func(env *Environment) []string {
		var out []string
		for _, svc := range env.activeServices() {
			out = append(out, svc.Name)
		}
		return out
	}

	assert.Equal(t, []string{ServiceWeb, ServiceDatabase, ServiceAdminer, ServiceMailer}, names(&Environment{}),
		"the base stack needs nothing from the lock")

	full := names(&Environment{
		features:        features{AMQP: true, Elasticsearch: true, RedisMessenger: true, S3: true},
		dedicatedWorker: true,
		php:             PHP{Profiler: ProfilerBlackfire, BlackfireServerID: "id", BlackfireServerToken: "token"},
	})
	assert.Equal(t, []string{
		ServiceWeb, ServiceDatabase, ServiceAdminer, ServiceMailer,
		ServiceWorker, ServiceScheduler,
		ServiceQueue, ServiceSearch, ServiceCache, ServiceStorage, ServiceStorageInit,
		ServiceBlackfire,
	}, full, "services appear in catalog order")

	// The cache follows needsRedis: redis messenger or S3.
	assert.Contains(t, names(&Environment{features: features{S3: true}}), ServiceCache)
	assert.Contains(t, names(&Environment{features: features{RedisMessenger: true}}), ServiceCache)

	// Profiler daemons need their credentials, not just the profiler choice.
	assert.NotContains(t, names(&Environment{php: PHP{Profiler: ProfilerBlackfire}}), ServiceBlackfire)
	assert.NotContains(t, names(&Environment{php: PHP{Profiler: ProfilerTideways}}), ServiceTideways)
	assert.Contains(t, names(&Environment{php: PHP{Profiler: ProfilerTideways, TidewaysAPIKey: "key"}}), ServiceTideways)
}

func TestLookups(t *testing.T) {
	t.Parallel()

	assert.Nil(t, byName("unknown"))
	mailer := byName(ServiceMailer)
	require.NotNil(t, mailer)
	assert.Equal(t, "Mailpit", mailer.Label)

	assert.Nil(t, mailer.endpointNamed("unknown"))
	assert.Nil(t, byName(ServiceCache).endpointNamed(""), "an empty name never matches a nameless endpoint")

	ep := mailer.endpointNamed(PortHTTP)
	require.NotNil(t, ep)
	assert.Equal(t, 8025, ep.ContainerPort)
	assert.Equal(t, 8025, ep.DefaultHostPort)
	assert.Equal(t, "mailer", ep.Subdomain)

	assert.Equal(t, 8000, mustEndpoint(ServiceWeb, PortHTTP).ContainerPort)
	assert.Panics(t, func() { mustEndpoint(ServiceWeb, "unknown") })
	assert.Panics(t, func() { mustEndpoint("unknown", PortHTTP) })
	assert.Equal(t, "http://127.0.0.1:8000", DefaultShopURL)
}

func TestConfigurableServices(t *testing.T) {
	t.Parallel()

	var names []string
	for _, svc := range configurableServices() {
		names = append(names, svc.Name)
	}
	assert.Equal(t, []string{ServiceWeb, ServiceDatabase, ServiceAdminer, ServiceMailer, ServiceQueue, ServiceSearch, ServiceStorage}, names,
		"services without a ports key or variants (cache, storage-init, console processes, profiler daemons) take no settings")
}

func TestEndpointHostPort(t *testing.T) {
	t.Parallel()

	web := mustEndpoint(ServiceWeb, PortHTTP)
	assert.Equal(t, 8000, web.hostPort(nil))
	assert.Equal(t, 8000, web.hostPort(Ports{}))
	assert.Equal(t, 8005, web.hostPort(Ports{PortHTTP: 8005}))
	assert.Equal(t, 0, web.hostPort(Ports{PortHTTP: PortDisabled}))
	assert.True(t, web.published(nil))
	assert.False(t, web.published(Ports{PortHTTP: PortDisabled}))

	// A nameless endpoint is never published, whatever the config says.
	internal := endpoint{ContainerPort: 6379, DefaultHostPort: 6379}
	assert.Equal(t, 0, internal.hostPort(nil))
	assert.False(t, internal.published(nil))

	// A random-port endpoint is published but has no fixed host port until
	// pinned.
	db := mustEndpoint(ServiceDatabase, PortMySQL)
	assert.True(t, db.published(nil))
	assert.Equal(t, 0, db.hostPort(nil))
	assert.Equal(t, 3306, db.hostPort(Ports{PortMySQL: 3306}))
}

func TestEndpointURL(t *testing.T) {
	t.Parallel()

	adminer := mustEndpoint(ServiceAdminer, PortHTTP)

	// Plain mode: default, override, disabled.
	assert.Equal(t, "http://127.0.0.1:9080", adminer.url("", nil))
	assert.Equal(t, "http://127.0.0.1:9999", adminer.url("", Ports{PortHTTP: 9999}))
	assert.Empty(t, adminer.url("", Ports{PortHTTP: PortDisabled}))

	// Proxy mode: routed at the endpoint's subdomain.
	assert.Equal(t, "https://adminer.my-shop.local", adminer.url("my-shop.local", nil))

	// An unrouted endpoint has no proxy URL; a random-port endpoint has no
	// fixed address in plain mode.
	assert.Empty(t, mustEndpoint(ServiceMailer, PortSMTP).url("my-shop.local", nil))
	assert.Empty(t, mustEndpoint(ServiceDatabase, PortMySQL).url("", nil))
}

func TestServiceVariant(t *testing.T) {
	t.Parallel()

	db := byName(ServiceDatabase)
	require.NotNil(t, db)
	assert.Equal(t, DatabaseMariaDB, db.variantNamed("").Name, "the first variant is the default")
	assert.Equal(t, "mysql", db.variantNamed(DatabaseMySQL).Image)
	assert.Nil(t, db.variantNamed("postgres"))
	assert.Equal(t, []string{DatabaseMariaDB, DatabaseMySQL}, db.variantNames())

	assert.Nil(t, byName(ServiceMailer).variantNamed(""), "services without variants have no default either")

	queue := byName(ServiceQueue)
	assert.Equal(t, QueueLavinMQ, queue.variantNamed("").Name)
	assert.Equal(t, "rabbitmq", queue.variantNamed(QueueRabbitMQ).Image)

	// selected resolves type and version from the settings with defaults.
	v, version := db.selected(&Environment{})
	assert.Equal(t, DatabaseMariaDB, v.Name)
	assert.Equal(t, "11.8", version)
	v, version = db.selected(&Environment{settings: Settings{ServiceDatabase: {Type: DatabaseMySQL, Version: "8.0"}}})
	assert.Equal(t, DatabaseMySQL, v.Name)
	assert.Equal(t, "8.0", version)
}

func TestServiceProxyURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "https://adminer.my-shop.local", byName(ServiceAdminer).proxyURL("my-shop.local"))
	// The storage UI endpoint is the console, routed at its own subdomain.
	assert.Equal(t, "https://storage.my-shop.local", byName(ServiceStorage).proxyURL("my-shop.local"))
	// Web's UI endpoint has no subdomain (its proxy routes are custom).
	assert.Empty(t, byName(ServiceWeb).proxyURL("my-shop.local"))
}

func TestServicePublishedURL(t *testing.T) {
	t.Parallel()

	storage := byName(ServiceStorage)
	require.NotNil(t, storage)

	// Plain mode: the published port of the UI endpoint (the console, 9001)
	// wins, even when other endpoints are published too.
	assert.Equal(t, "http://127.0.0.1:19001", storage.publishedURL(map[int]int{9000: 19000, 9001: 19001}, ""))
	// Proxy mode: the console subdomain, not the S3 API one.
	assert.Equal(t, "https://storage.my-shop.local", storage.publishedURL(nil, "my-shop.local"))
	// Neither published nor proxied: unreachable.
	assert.Empty(t, storage.publishedURL(nil, ""))

	// Services without a UI endpoint never get a URL.
	assert.Empty(t, byName(ServiceCache).publishedURL(map[int]int{6379: 16379}, ""))
}

func TestServiceLink(t *testing.T) {
	t.Parallel()

	link, ok := ServiceLink(ServiceAdminer, "my-shop.local")
	require.True(t, ok)
	assert.Equal(t, Link{Label: "Adminer", URL: "https://adminer.my-shop.local"}, link)

	link, ok = ServiceLink(ServiceStorage, "my-shop.local")
	require.True(t, ok)
	assert.Equal(t, Link{Label: "Storage (S3)", URL: "https://storage.my-shop.local"}, link)

	_, ok = ServiceLink(ServiceWeb, "my-shop.local")
	assert.False(t, ok, "hidden services have no link")
	_, ok = ServiceLink("unknown", "my-shop.local")
	assert.False(t, ok)
}

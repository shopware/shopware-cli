package docker

import (
	"fmt"
	"net"
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActivePublishedEndpoints(t *testing.T) {
	t.Parallel()

	keysOf := func(services []service) []string {
		var keys []string
		for _, svc := range services {
			for _, ep := range svc.publishedEndpoints() {
				keys = append(keys, svc.Name+"."+ep.Name)
			}
		}
		return keys
	}

	base := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}
	keys := keysOf(envFor(base, Environment{}).activeServices())
	assert.Contains(t, keys, "web.http")
	assert.Contains(t, keys, "database.mysql")
	assert.NotContains(t, keys, "queue.amqp")
	assert.NotContains(t, keys, "search.http")
	assert.NotContains(t, keys, "storage.s3")

	full := &composer.Lock{Packages: []composer.LockPackage{
		{Name: "shopware/core", Version: "6.6.0.0"},
		{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
		{Name: "shopware/elasticsearch", Version: "6.6.0.0"},
		{Name: "shopware/k8s-meta", Version: "1.0.0"},
	}}
	keys = keysOf(envFor(full, Environment{}).activeServices())
	assert.ElementsMatch(t, []string{
		"web.http", "web.http_alt", "web.storefront_watcher_assets", "web.storefront_watcher", "web.admin_watcher", "web.admin_watcher_hmr",
		"database.mysql",
		"adminer.http",
		"mailer.smtp", "mailer.http",
		"queue.management", "queue.amqp",
		"search.http",
		"storage.s3", "storage.console",
	}, keys, "a lock with every optional package activates every catalog port")
}

// freeExcept returns an isFree predicate that reports the given ports as busy.
func freeExcept(ports ...int) func(int) bool {
	set := map[int]struct{}{}
	for _, p := range ports {
		set[p] = struct{}{}
	}
	return func(port int) bool {
		_, isBusy := set[port]
		return !isBusy
	}
}

func TestFindConflicts(t *testing.T) {
	t.Parallel()

	webPorts := func(port Port) Settings {
		return Settings{ServiceWeb: {Ports: Ports{PortHTTP: port}}}
	}

	t.Run("busy port is reported with its service and endpoint", func(t *testing.T) {
		t.Parallel()
		conflicts := (&Environment{}).findConflicts(nil, freeExcept(8000))
		require.Len(t, conflicts, 1)
		assert.Equal(t, PortConflict{Service: ServiceWeb, Endpoint: PortHTTP, Label: "Shop (Caddy)", HostPort: 8000}, conflicts[0])
		assert.Equal(t, "docker.services.web.ports.http", conflicts[0].ConfigPath())
	})

	t.Run("nameless endpoint is never probed", func(t *testing.T) {
		t.Parallel()
		env := &Environment{features: features{S3: true}}
		assert.Empty(t, env.findConflicts(nil, freeExcept(6379)), "the cache has no ports key and is not published by the generator")
	})

	t.Run("random-port endpoint cannot conflict unless pinned", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, (&Environment{}).findConflicts(nil, freeExcept(3306)))

		pinned := &Environment{settings: Settings{ServiceDatabase: {Ports: Ports{PortMySQL: 3306}}}}
		conflicts := pinned.findConflicts(nil, freeExcept(3306))
		require.Len(t, conflicts, 1)
		assert.Equal(t, ServiceDatabase, conflicts[0].Service)
	})

	t.Run("port published by own containers is not a conflict", func(t *testing.T) {
		t.Parallel()
		owned := map[int]struct{}{8000: {}}
		assert.Empty(t, (&Environment{}).findConflicts(owned, freeExcept(8000)))
	})

	t.Run("configured override is probed instead of the default", func(t *testing.T) {
		t.Parallel()
		env := &Environment{settings: webPorts(9500)}
		assert.Empty(t, env.findConflicts(nil, freeExcept(8000)), "default 8000 busy, but web is remapped to free 9500")

		conflicts := env.findConflicts(nil, freeExcept(9500))
		require.Len(t, conflicts, 1)
		assert.Equal(t, 9500, conflicts[0].HostPort)
	})

	t.Run("disabled port is never probed", func(t *testing.T) {
		t.Parallel()
		env := &Environment{settings: webPorts(PortDisabled)}
		assert.Empty(t, env.findConflicts(nil, freeExcept(8000)), "a disabled port is not published and cannot conflict")
	})

	t.Run("proxy mode probes only unrouted services", func(t *testing.T) {
		t.Parallel()
		env := &Environment{
			proxy:    &Proxy{Hostname: "my-shop.shopware.local"},
			settings: Settings{ServiceDatabase: {Ports: Ports{PortMySQL: 3306}}},
		}
		conflicts := env.findConflicts(nil, freeExcept(8000, 9080, 3306))
		require.Len(t, conflicts, 1, "routed services publish nothing; only the pinned database port is probed")
		assert.Equal(t, "docker.services.database.ports.mysql", conflicts[0].ConfigPath())
	})

	t.Run("all free means no conflicts", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, (&Environment{}).findConflicts(nil, freeExcept()))
	})
}

func TestPortConflictsCoversS3Ports(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeLock(t, dir, "shopware/k8s-meta")

	// Occupy a random port and configure it as the S3 host port, so the probe
	// reports it as busy regardless of what else runs on this machine.
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", ":0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	busyPort := listener.Addr().(*net.TCPAddr).Port

	env, err := NewEnvironment(dir, Options{Services: Settings{
		ServiceStorage: {Ports: Ports{PortS3: Port(busyPort)}},
	}})
	require.NoError(t, err)

	paths := make([]string, 0)
	for _, conflict := range env.PortConflicts(t.Context()) {
		paths = append(paths, conflict.ConfigPath())
	}
	assert.Contains(t, paths, "docker.services.storage.ports.s3", "storage ports must be probed for conflicts with an S3 lock")
}

func TestIsPortFree(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", ":0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port

	assert.False(t, isPortFree(ctx, port), "port held by a listener must report busy")

	require.NoError(t, listener.Close())
	assert.True(t, isPortFree(ctx, port), "released port must report free")
}

func TestAllocateRandomPorts(t *testing.T) {
	t.Parallel()

	conflicts := []PortConflict{
		{Service: ServiceWeb, Endpoint: PortHTTP, Label: "Shop (Caddy)", HostPort: 8000},
		{Service: ServiceMailer, Endpoint: PortHTTP, Label: "Mailpit UI", HostPort: 8025},
		{Service: ServiceAdminer, Endpoint: PortHTTP, Label: "Adminer", HostPort: 9080},
	}

	overrides, err := AllocateRandomPorts(t.Context(), conflicts)
	require.NoError(t, err)
	require.Len(t, overrides, 3)

	seen := map[int]string{}
	for i, o := range overrides {
		key := o.Service + "." + o.Endpoint
		assert.Equal(t, conflicts[i].Service, o.Service, "overrides come back in conflict order")
		assert.Equal(t, conflicts[i].Endpoint, o.Endpoint)
		assert.Greater(t, o.HostPort, 0, "port for %s", key)
		if firstKey, dup := seen[o.HostPort]; dup {
			t.Fatalf("port %d handed out twice: %s and %s", o.HostPort, firstKey, key)
		}
		seen[o.HostPort] = key

		listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", fmt.Sprintf(":%d", o.HostPort))
		require.NoError(t, err, "allocated port %d for %s must be bindable", o.HostPort, key)
		require.NoError(t, listener.Close())
	}
}

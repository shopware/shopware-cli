package docker

import (
	"fmt"
	"net"
	"testing"

	"github.com/shyim/go-composer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestHostPort(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 8000, HostPort(nil, shop.DockerPortWeb))
	assert.Equal(t, 9080, HostPort(shop.ConfigDockerPorts{}, shop.DockerPortAdminer))
	assert.Equal(t, 8005, HostPort(shop.ConfigDockerPorts{shop.DockerPortWeb: 8005}, shop.DockerPortWeb))
	assert.Equal(t, 0, HostPort(shop.ConfigDockerPorts{shop.DockerPortWeb: shop.DockerPortDisabled}, shop.DockerPortWeb))
	assert.Equal(t, 0, HostPort(nil, "unknown"))
}

func TestActiveDefinitions(t *testing.T) {
	t.Parallel()

	keysOf := func(defs []PortDefinition) []string {
		keys := make([]string, 0, len(defs))
		for _, def := range defs {
			keys = append(keys, def.Key)
		}
		return keys
	}

	base := &composer.Lock{Packages: []composer.LockPackage{{Name: "shopware/core", Version: "6.6.0.0"}}}
	keys := keysOf(activeDefinitions(base))
	assert.Contains(t, keys, shop.DockerPortWeb)
	assert.NotContains(t, keys, shop.DockerPortAMQP)
	assert.NotContains(t, keys, shop.DockerPortAMQPManagement)
	assert.NotContains(t, keys, shop.DockerPortElasticsearch)

	full := &composer.Lock{Packages: []composer.LockPackage{
		{Name: "shopware/core", Version: "6.6.0.0"},
		{Name: "symfony/amqp-messenger", Version: "v7.0.0"},
		{Name: "shopware/elasticsearch", Version: "6.6.0.0"},
	}}
	keys = keysOf(activeDefinitions(full))
	assert.Contains(t, keys, shop.DockerPortAMQP)
	assert.Contains(t, keys, shop.DockerPortAMQPManagement)
	assert.Contains(t, keys, shop.DockerPortElasticsearch)
	assert.Len(t, keys, len(PortDefinitions))
}

func TestFindConflicts(t *testing.T) {
	t.Parallel()

	defs := []PortDefinition{
		{Key: shop.DockerPortWeb, Service: "web", Label: "Shop", Target: 8000, Default: 8000},
		{Key: shop.DockerPortMailerWeb, Service: "mailer", Label: "Mailpit UI", Target: 8025, Default: 8025},
	}

	busy := func(ports ...int) func(int) bool {
		set := map[int]struct{}{}
		for _, p := range ports {
			set[p] = struct{}{}
		}
		return func(port int) bool {
			_, isBusy := set[port]
			return !isBusy
		}
	}

	t.Run("busy port is reported with its definition", func(t *testing.T) {
		t.Parallel()
		conflicts := findConflicts(defs, nil, nil, busy(8000))
		require.Len(t, conflicts, 1)
		assert.Equal(t, shop.DockerPortWeb, conflicts[0].Definition.Key)
		assert.Equal(t, 8000, conflicts[0].HostPort)
	})

	t.Run("port published by own containers is not a conflict", func(t *testing.T) {
		t.Parallel()
		owned := map[int]struct{}{8000: {}}
		conflicts := findConflicts(defs, nil, owned, busy(8000))
		assert.Empty(t, conflicts)
	})

	t.Run("configured override is probed instead of the default", func(t *testing.T) {
		t.Parallel()
		ports := shop.ConfigDockerPorts{shop.DockerPortWeb: 9500}
		conflicts := findConflicts(defs, ports, nil, busy(8000))
		assert.Empty(t, conflicts, "default 8000 busy, but web is remapped to free 9500")

		conflicts = findConflicts(defs, ports, nil, busy(9500))
		require.Len(t, conflicts, 1)
		assert.Equal(t, 9500, conflicts[0].HostPort)
	})

	t.Run("disabled port is never probed", func(t *testing.T) {
		t.Parallel()
		ports := shop.ConfigDockerPorts{shop.DockerPortWeb: shop.DockerPortDisabled}
		conflicts := findConflicts(defs, ports, nil, busy(8000))
		assert.Empty(t, conflicts, "a disabled port is not published and cannot conflict")
	})

	t.Run("all free means no conflicts", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, findConflicts(defs, nil, nil, busy()))
	})
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
		{Definition: PortDefinition{Key: shop.DockerPortWeb, Target: 8000, Default: 8000}, HostPort: 8000},
		{Definition: PortDefinition{Key: shop.DockerPortMailerWeb, Target: 8025, Default: 8025}, HostPort: 8025},
		{Definition: PortDefinition{Key: shop.DockerPortAdminer, Target: 8080, Default: 9080}, HostPort: 9080},
	}

	ports, err := AllocateRandomPorts(t.Context(), conflicts)
	require.NoError(t, err)
	require.Len(t, ports, 3)

	seen := map[int]string{}
	for key, port := range ports {
		assert.Greater(t, port, 0, "port for %s", key)
		if firstKey, dup := seen[port]; dup {
			t.Fatalf("port %d handed out twice: %s and %s", port, firstKey, key)
		}
		seen[port] = key

		listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", fmt.Sprintf(":%d", port))
		require.NoError(t, err, "allocated port %d for %s must be bindable", port, key)
		require.NoError(t, listener.Close())
	}
}

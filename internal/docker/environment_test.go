package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfilerIsPaid(t *testing.T) {
	t.Parallel()

	assert.False(t, ProfilerIsPaid(""))
	assert.False(t, ProfilerIsPaid(ProfilerXdebug))
	assert.False(t, ProfilerIsPaid(ProfilerPcov))
	assert.False(t, ProfilerIsPaid(ProfilerSpx))
	assert.True(t, ProfilerIsPaid(ProfilerBlackfire))
	assert.True(t, ProfilerIsPaid(ProfilerTideways))
}

func TestPHPWebImage(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "ghcr.io/shopware/docker-dev:php8.3-node24-caddy", PHP{}.WebImage(), "defaults to PHP 8.3")
	assert.Equal(t, "ghcr.io/shopware/docker-dev:php8.2-node24-caddy", PHP{Version: "8.2"}.WebImage())
}

func TestNewEnvironment(t *testing.T) {
	t.Parallel()

	t.Run("requires a readable composer.lock", func(t *testing.T) {
		t.Parallel()
		_, err := NewEnvironment(t.TempDir(), Options{})
		assert.ErrorContains(t, err, "composer.lock")
	})

	t.Run("reads the optional services from the lock", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeLock(t, dir, "symfony/amqp-messenger", "shopware/k8s-meta")

		env, err := NewEnvironment(dir, Options{})
		require.NoError(t, err)
		assert.Equal(t, features{AMQP: true, S3: true}, env.features)
		assert.False(t, env.dedicatedWorker, "the admin worker is enabled unless configured off")
		assert.Empty(t, env.ProxyHost())
		assert.Equal(t, "ghcr.io/shopware/docker-dev:php8.3-node24-caddy", env.WebImage())
	})

	t.Run("adds the console processes when the admin worker is disabled", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeLock(t, dir)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "config", "packages"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "config", "packages", "shopware.yaml"), []byte("shopware:\n  admin_worker:\n    enable_admin_worker: false\n"), 0o644))

		env, err := NewEnvironment(dir, Options{})
		require.NoError(t, err)
		assert.True(t, env.dedicatedWorker)
	})

	t.Run("carries the options", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeLock(t, dir)

		env, err := NewEnvironment(dir, Options{
			PHP:      PHP{Version: "8.2"},
			Services: Settings{ServiceWeb: {Ports: Ports{PortHTTP: 8005}}},
			Proxy:    &Proxy{Hostname: "my-shop.shopware.local"},
			User:     "1001:46",
		})
		require.NoError(t, err)
		assert.Equal(t, "ghcr.io/shopware/docker-dev:php8.2-node24-caddy", env.WebImage())
		assert.Equal(t, Ports{PortHTTP: 8005}, env.ports(ServiceWeb))
		assert.Equal(t, "my-shop.shopware.local", env.ProxyHost())
		assert.Equal(t, "1001:46", env.user)
	})
}

func TestWatchURLs(t *testing.T) {
	t.Parallel()

	plain := &Environment{}
	assert.Equal(t, "http://127.0.0.1:5173", plain.AdminWatchURL())
	assert.Equal(t, "http://127.0.0.1:9998", plain.StorefrontWatchURL())

	remapped := &Environment{settings: Settings{ServiceWeb: {Ports: Ports{
		PortAdminWatcher:      15173,
		PortStorefrontWatcher: PortDisabled,
	}}}}
	assert.Equal(t, "http://127.0.0.1:15173", remapped.AdminWatchURL(), "overrides are honored")
	assert.Empty(t, remapped.StorefrontWatchURL(), "a disabled port has no URL")

	proxied := &Environment{proxy: &Proxy{Hostname: "my-shop.shopware.local"}}
	assert.Equal(t, "https://admin-watch.my-shop.shopware.local", proxied.AdminWatchURL())
	assert.Equal(t, "https://storefront-watch.my-shop.shopware.local", proxied.StorefrontWatchURL())
}

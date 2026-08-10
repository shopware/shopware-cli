package proxy

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestLocalDomainHostname(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "my-shop.shopware.local", LocalDomainHostname("my-shop", "shopware.local"))
	// Only the final path element matters.
	assert.Equal(t, "my-shop.shopware.local", LocalDomainHostname("/tmp/projects/my-shop", "shopware.local"))
	// Underscores are valid in a project name but not a hostname → dashes.
	assert.Equal(t, "my-shop.shopware.local", LocalDomainHostname("my_shop", "shopware.local"))
	// Custom base domain is respected.
	assert.Equal(t, "my-shop.dev.internal", LocalDomainHostname("my-shop", "dev.internal"))
}

func TestProjectHostname(t *testing.T) {
	t.Parallel()

	t.Run("derives from directory name when no url is configured", func(t *testing.T) {
		t.Parallel()

		root := filepath.Join(t.TempDir(), "my-shop")
		hostname, err := ProjectHostname(root, &shop.Config{}, "shopware.local")
		assert.NoError(t, err)
		assert.Equal(t, "my-shop.shopware.local", hostname)
	})

	t.Run("maps underscores in the directory name to dashes for a valid dns label", func(t *testing.T) {
		t.Parallel()

		root := filepath.Join(t.TempDir(), "my_shop")
		hostname, err := ProjectHostname(root, &shop.Config{}, "shopware.local")
		assert.NoError(t, err)
		assert.Equal(t, "my-shop.shopware.local", hostname)
	})

	t.Run("rejects a directory name that is not a valid dns label", func(t *testing.T) {
		t.Parallel()

		root := filepath.Join(t.TempDir(), "My Shop!")
		_, err := ProjectHostname(root, &shop.Config{}, "shopware.local")
		assert.Error(t, err)
	})

	t.Run("uses the configured url host as an override", func(t *testing.T) {
		t.Parallel()

		root := filepath.Join(t.TempDir(), "my-shop")
		hostname, err := ProjectHostname(root, &shop.Config{URL: "https://custom-name.shopware.local:8443"}, "shopware.local")
		assert.NoError(t, err)
		assert.Equal(t, "custom-name.shopware.local", hostname)
	})

	t.Run("ignores IP and localhost urls from default configs", func(t *testing.T) {
		t.Parallel()

		root := filepath.Join(t.TempDir(), "my-shop")
		for _, u := range []string{"http://127.0.0.1:8000", "http://localhost:8000", "http://[::1]:8000"} {
			hostname, err := ProjectHostname(root, &shop.Config{URL: u}, "shopware.local")
			assert.NoError(t, err, u)
			assert.Equal(t, "my-shop.shopware.local", hostname, u)
		}
	})

	t.Run("nil config falls back to directory name", func(t *testing.T) {
		t.Parallel()

		root := filepath.Join(t.TempDir(), "my-shop")
		hostname, err := ProjectHostname(root, nil, "shopware.local")
		assert.NoError(t, err)
		assert.Equal(t, "my-shop.shopware.local", hostname)
	})
}

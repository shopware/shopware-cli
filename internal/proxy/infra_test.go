package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestIsProxyProjectForDomain(t *testing.T) {
	t.Parallel()

	proxied := func(url string) *shop.Config {
		return &shop.Config{URL: url}
	}

	assert.True(t, IsProxyProjectForDomain(proxied("https://my-shop.shopware.local"), "shopware.local"))
	assert.True(t, IsProxyProjectForDomain(proxied("https://shopware.local"), "shopware.local"))
	assert.True(t, IsProxyProjectForDomain(proxied("https://my-shop.dev.internal"), "dev.internal"))

	// The local environment url overrides the top-level one.
	envProxied := &shop.Config{
		URL:          "http://127.0.0.1:8000",
		Environments: map[string]*shop.EnvironmentConfig{"local": {URL: "https://my-shop.shopware.local"}},
	}
	assert.True(t, IsProxyProjectForDomain(envProxied, "shopware.local"))

	// Port-based projects are not proxy projects.
	assert.False(t, IsProxyProjectForDomain(proxied("http://127.0.0.1:8000"), "shopware.local"))
	assert.False(t, IsProxyProjectForDomain(proxied("http://localhost:8000"), "shopware.local"))
	// Hostname under a different domain is not matched.
	assert.False(t, IsProxyProjectForDomain(proxied("https://my-shop.example.com"), "shopware.local"))
	// A hostname that merely ends with the domain as a substring (no dot) must not match.
	assert.False(t, IsProxyProjectForDomain(proxied("https://notshopware.local"), "shopware.local"))
	assert.False(t, IsProxyProjectForDomain(nil, "shopware.local"))
	assert.False(t, IsProxyProjectForDomain(&shop.Config{}, "shopware.local"))
}

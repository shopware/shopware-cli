package project

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestLocalDomainHostname(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "my-shop.shopware.local", localDomainHostname("my-shop", "shopware.local"))
	// Only the final path element matters.
	assert.Equal(t, "my-shop.shopware.local", localDomainHostname("/tmp/projects/my-shop", "shopware.local"))
	// Underscores are valid in a project name but not a hostname → dashes.
	assert.Equal(t, "my-shop.shopware.local", localDomainHostname("my_shop", "shopware.local"))
	// Custom base domain is respected.
	assert.Equal(t, "my-shop.dev.internal", localDomainHostname("my-shop", "dev.internal"))
}

func TestIsProxyProjectForDomain(t *testing.T) {
	t.Parallel()

	proxied := func(url string) *shop.Config {
		return &shop.Config{URL: url}
	}

	assert.True(t, isProxyProjectForDomain(proxied("https://my-shop.shopware.local"), "shopware.local"))
	assert.True(t, isProxyProjectForDomain(proxied("https://shopware.local"), "shopware.local"))
	assert.True(t, isProxyProjectForDomain(proxied("https://my-shop.dev.internal"), "dev.internal"))

	// Port-based projects are not proxy projects.
	assert.False(t, isProxyProjectForDomain(proxied("http://127.0.0.1:8000"), "shopware.local"))
	assert.False(t, isProxyProjectForDomain(proxied("http://localhost:8000"), "shopware.local"))
	// Hostname under a different domain is not matched.
	assert.False(t, isProxyProjectForDomain(proxied("https://my-shop.example.com"), "shopware.local"))
	// A hostname that merely ends with the domain as a substring (no dot) must not match.
	assert.False(t, isProxyProjectForDomain(proxied("https://notshopware.local"), "shopware.local"))
	assert.False(t, isProxyProjectForDomain(nil, "shopware.local"))
	assert.False(t, isProxyProjectForDomain(&shop.Config{}, "shopware.local"))
}

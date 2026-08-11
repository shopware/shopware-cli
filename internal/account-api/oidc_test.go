package account_api

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsStaging(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")
	assert.False(t, isStaging())

	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "1")
	assert.True(t, isStaging())
}

func TestGetOIDCEndpoint(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_OIDC_ENDPOINT", "https://custom.example")
	assert.Equal(t, "https://custom.example", getOIDCEndpoint())

	t.Setenv("SHOPWARE_CLI_OIDC_ENDPOINT", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "1")
	assert.Equal(t, "https://auth-api.shopware.in", getOIDCEndpoint())

	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")
	assert.Equal(t, "https://auth-api.shopware.com", getOIDCEndpoint())
}

func TestGetOIDCClientID(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "1")
	assert.Equal(t, "def413d7-4c4e-439f-8b51-74c352436b2f", getOIDCClientID())

	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")
	assert.Equal(t, "069d0a55-5237-4706-a5c9-7cb1a45f1e81", getOIDCClientID())
}

func TestGetApiUrl(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", "https://api.test.local")
	assert.Equal(t, "https://api.test.local", getApiUrl())

	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "1")
	assert.Equal(t, "https://next-api.shopware.com", getApiUrl())

	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")
	assert.Equal(t, "https://api.shopware.com", getApiUrl())
}

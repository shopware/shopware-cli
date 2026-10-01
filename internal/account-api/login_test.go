package account_api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

	"github.com/shopware/shopware-cli/internal/system"
)

func TestNewApiUsesClientCredentialsFromEnv(t *testing.T) {
	var tokenRequested atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			tokenRequested.Store(true)

			assert.Equal(t, "client_credentials", r.FormValue("grant_type"))
			assert.Equal(t, "test-client-id", r.FormValue("client_id"))
			assert.Equal(t, "test-client-secret", r.FormValue("client_secret"))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": "test-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_ID", "test-client-id")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET", "test-client-secret")
	t.Setenv("SHOPWARE_CLI_OIDC_ENDPOINT", srv.URL)

	client, err := NewApi(t.Context())
	assert.NoError(t, err)
	assert.NotNil(t, client)
	assert.True(t, tokenRequested.Load(), "expected token endpoint to be called")
	assert.NotNil(t, client.Token)
	assert.Equal(t, "test-token", client.Token.AccessToken)
}

func TestNewApiFailsWithIncompleteClientCredentials(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_ID", "test-client-id")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_EMAIL", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_PASSWORD", "")

	_, err := NewApi(t.Context())
	assert.ErrorContains(t, err, "both SHOPWARE_CLI_ACCOUNT_CLIENT_ID and SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET must be set")
}

func TestNewApiUsesValidTokenCache(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_ID", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_EMAIL", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_PASSWORD", "")

	cached := &Client{Token: &oauth2.Token{AccessToken: "from-cache", Expiry: time.Now().Add(time.Hour)}}
	require.NoError(t, saveApiTokenToTokenCache(cached))

	client, err := NewApi(t.Context())
	require.NoError(t, err)
	require.NotNil(t, client.Token)
	assert.Equal(t, "from-cache", client.Token.AccessToken)
}

func TestNewApiFailsWithoutLoginWhenInteractionDisabled(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_ID", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_EMAIL", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_PASSWORD", "")

	_, err := NewApi(system.WithInteraction(t.Context(), false))

	assert.ErrorContains(t, err, "shopware-cli account login")
	assert.ErrorContains(t, err, "SHOPWARE_CLI_ACCOUNT_CLIENT_ID")
}

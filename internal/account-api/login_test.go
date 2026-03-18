package account_api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
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

	_, err := NewApi(t.Context())
	assert.ErrorContains(t, err, "both SHOPWARE_CLI_ACCOUNT_CLIENT_ID and SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET must be set")
}

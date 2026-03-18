package account_api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewApiUsesClientCredentialsFromEnv(t *testing.T) {
	tokenRequested := false

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth2/token" {
			tokenRequested = true

			assert.Equal(t, "client_credentials", r.FormValue("grant_type"))

			// client credentials may be sent via Basic auth header or form values
			user, pass, hasBasicAuth := r.BasicAuth()
			if hasBasicAuth {
				assert.Equal(t, "test-client-id", user)
				assert.Equal(t, "test-client-secret", pass)
			} else {
				assert.Equal(t, "test-client-id", r.FormValue("client_id"))
				assert.Equal(t, "test-client-secret", r.FormValue("client_secret"))
			}

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
	assert.True(t, tokenRequested, "expected token endpoint to be called")
	assert.NotNil(t, client.Token)
	assert.Equal(t, "test-token", client.Token.AccessToken)
}

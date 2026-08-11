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

func TestNewApiUsesLegacyCredentials(t *testing.T) {
	var loginCalled atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accesstokens" {
			loginCalled.Store(true)
			assert.Equal(t, http.MethodPost, r.Method)

			var body loginRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "user@example.com", body.Email)
			assert.Equal(t, "secret", body.Password)

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(legacyToken{
				Token: "legacy-access",
				Expire: tokenExpire{
					Date:     time.Now().UTC().Add(time.Hour).Format("2006-01-02 15:04:05.000000"),
					Timezone: "UTC",
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", srv.URL)
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_ID", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET", "")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_EMAIL", "user@example.com")
	t.Setenv("SHOPWARE_CLI_ACCOUNT_PASSWORD", "secret")

	client, err := NewApi(t.Context())
	require.NoError(t, err)
	assert.True(t, loginCalled.Load())
	require.NotNil(t, client.LegacyToken)
	assert.Equal(t, "legacy-access", client.LegacyToken.Token)
}

func TestLoginWithCredentialsFailureDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"detail":"Invalid credentials"}`))
	}))
	defer srv.Close()

	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", srv.URL)

	_, err := loginWithCredentials(t.Context(), "a@b.c", "wrong")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Invalid credentials")
}

func TestLoginWithCredentialsGenericFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", srv.URL)

	_, err := loginWithCredentials(t.Context(), "a@b.c", "wrong")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "login failed. Check your credentials")
}

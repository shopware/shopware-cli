package account_api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestSetUserAgent(t *testing.T) {
	prev := httpUserAgent
	t.Cleanup(func() { httpUserAgent = prev })

	SetUserAgent("shopware-cli/test")
	assert.Equal(t, "shopware-cli/test", httpUserAgent)
}

func TestNewAuthenticatedRequestWithOAuthToken(t *testing.T) {
	prev := httpUserAgent
	t.Cleanup(func() { httpUserAgent = prev })
	SetUserAgent("shopware-cli/test")

	client := &Client{
		Token: &oauth2.Token{AccessToken: "access-token"},
	}

	req, err := client.NewAuthenticatedRequest(t.Context(), http.MethodGet, "https://example.com/path", nil)
	require.NoError(t, err)
	assert.Equal(t, "application/json", req.Header.Get("content-type"))
	assert.Equal(t, "application/json", req.Header.Get("accept"))
	assert.Equal(t, "Bearer access-token", req.Header.Get("Authorization"))
	assert.Equal(t, "shopware-cli/test", req.Header.Get("user-agent"))
	assert.Empty(t, req.Header.Get("x-shopware-token"))
}

func TestNewAuthenticatedRequestWithLegacyToken(t *testing.T) {
	client := &Client{
		LegacyToken: &legacyToken{Token: "legacy-token"},
	}

	req, err := client.NewAuthenticatedRequest(t.Context(), http.MethodPost, "https://example.com/path", nil)
	require.NoError(t, err)
	assert.Equal(t, "legacy-token", req.Header.Get("x-shopware-token"))
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestDoRequestSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client := &Client{}
	req, err := client.NewAuthenticatedRequest(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	body, err := client.doRequest(req)
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":true}`, string(body))
}

func TestDoRequestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"detail":"bad request"}`))
	}))
	defer srv.Close()

	client := &Client{}
	req, err := client.NewAuthenticatedRequest(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	_, err = client.doRequest(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "got status code 400")
	assert.Contains(t, err.Error(), "bad request")
}

func TestIsTokenValidOAuth(t *testing.T) {
	assert.True(t, (&Client{Token: &oauth2.Token{Expiry: time.Now().Add(time.Hour)}}).isTokenValid())
	assert.False(t, (&Client{Token: &oauth2.Token{Expiry: time.Now().Add(30 * time.Second)}}).isTokenValid())
	assert.False(t, (&Client{}).isTokenValid())
}

func TestIsTokenValidLegacy(t *testing.T) {
	valid := &Client{LegacyToken: &legacyToken{
		Token: "tok",
		Expire: tokenExpire{
			Date:     time.Now().UTC().Add(time.Hour).Format("2006-01-02 15:04:05.000000"),
			Timezone: "UTC",
		},
	}}
	assert.True(t, valid.isTokenValid())

	expired := &Client{LegacyToken: &legacyToken{
		Token: "tok",
		Expire: tokenExpire{
			Date:     time.Now().UTC().Add(-time.Hour).Format("2006-01-02 15:04:05.000000"),
			Timezone: "UTC",
		},
	}}
	assert.False(t, expired.isTokenValid())

	badTimezone := &Client{LegacyToken: &legacyToken{
		Expire: tokenExpire{Date: "2020-01-01 00:00:00.000000", Timezone: "Not/AZone"},
	}}
	assert.False(t, badTimezone.isTokenValid())

	badDate := &Client{LegacyToken: &legacyToken{
		Expire: tokenExpire{Date: "not-a-date", Timezone: "UTC"},
	}}
	assert.False(t, badDate.isTokenValid())
}

func TestGetCacheFileName(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")
	assert.Equal(t, "shopware-api-token.json", getCacheFileName())

	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "1")
	assert.Equal(t, "shopware-api-token-staging.json", getCacheFileName())
}

func TestCreateApiFromTokenCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", cacheDir)
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")

	t.Run("missing file", func(t *testing.T) {
		_, err := createApiFromTokenCache(t.Context())
		require.Error(t, err)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("invalid json", func(t *testing.T) {
		require.NoError(t, os.WriteFile(getApiTokenCacheFilePath(), []byte("not-json"), 0o600))
		_, err := createApiFromTokenCache(t.Context())
		require.Error(t, err)
	})

	t.Run("expired token", func(t *testing.T) {
		client := &Client{Token: &oauth2.Token{AccessToken: "x", Expiry: time.Now().Add(-time.Hour)}}
		content, err := json.Marshal(client)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(getApiTokenCacheFilePath(), content, 0o600))

		_, err = createApiFromTokenCache(t.Context())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "token is expired")
	})

	t.Run("valid token", func(t *testing.T) {
		client := &Client{Token: &oauth2.Token{AccessToken: "cached", Expiry: time.Now().Add(time.Hour)}}
		content, err := json.Marshal(client)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(getApiTokenCacheFilePath(), content, 0o600))

		got, err := createApiFromTokenCache(t.Context())
		require.NoError(t, err)
		require.NotNil(t, got.Token)
		assert.Equal(t, "cached", got.Token.AccessToken)
	})
}

func TestSaveAndInvalidateTokenCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", cacheDir)
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")

	// nested path so MkdirAll branch is exercised
	nested := filepath.Join(cacheDir, "nested")
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", nested)

	client := &Client{Token: &oauth2.Token{AccessToken: "save-me", Expiry: time.Now().Add(time.Hour)}}
	require.NoError(t, saveApiTokenToTokenCache(client))

	_, err := os.Stat(getApiTokenCacheFilePath())
	require.NoError(t, err)

	require.NoError(t, InvalidateTokenCache())
	_, err = os.Stat(getApiTokenCacheFilePath())
	assert.True(t, os.IsNotExist(err))

	// missing file is a no-op
	require.NoError(t, InvalidateTokenCache())
}

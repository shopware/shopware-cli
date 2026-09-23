package admin_sdk

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isolateTokenCache(t *testing.T) {
	t.Helper()
	t.Setenv(tokenCacheDirEnv, t.TempDir())
}

func newShopServer(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()
	isolateTokenCache(t)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/oauth/token" {
			_, _ = w.Write([]byte(`{"access_token":"test-token","token_type":"Bearer","expires_in":3600}`))
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/_info/config" {
			_, _ = w.Write([]byte(`{"version":"6.6.5.0","bundles":{}}`))
			return
		}
		if handler != nil {
			handler(w, r)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
}

func TestNewApiClientAuthenticatesAndLoadsVersion(t *testing.T) {
	srv := newShopServer(t, nil)
	t.Cleanup(srv.Close)

	client, err := NewApiClient(t.Context(), srv.URL, NewIntegrationCredentials("id", "secret"), srv.Client())
	require.NoError(t, err)
	require.NotNil(t, client)
	require.NotNil(t, client.ShopwareVersion)
	assert.Equal(t, "6.6.5.0", client.ShopwareVersion.String())

	token, err := client.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "test-token", token)
}

func TestNewApiClientPasswordGrant(t *testing.T) {
	isolateTokenCache(t)
	var grant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost && r.URL.Path == "/api/oauth/token" {
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			var payload map[string]string
			require.NoError(t, json.Unmarshal(body, &payload))
			grant = payload["grant_type"]
			assert.Equal(t, "administration", payload["client_id"])
			assert.Equal(t, "admin", payload["username"])
			_, _ = w.Write([]byte(`{"access_token":"pw-token","token_type":"Bearer","expires_in":3600}`))
			return
		}
		if r.URL.Path == "/api/_info/config" {
			_, _ = w.Write([]byte(`{"version":"6.6.5.0"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client, err := NewApiClient(t.Context(), srv.URL, NewPasswordCredentials("admin", "shopware"), srv.Client())
	require.NoError(t, err)
	assert.Equal(t, "password", grant)
	token, err := client.AccessToken(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "pw-token", token)
}

func TestNewApiClientAuthError(t *testing.T) {
	isolateTokenCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errors":[{"detail":"invalid credentials"}]}`))
	}))
	t.Cleanup(srv.Close)

	_, err := NewApiClient(t.Context(), srv.URL, NewIntegrationCredentials("id", "secret"), srv.Client())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid credentials")
}

func TestClearCache(t *testing.T) {
	var method, path string
	srv := newShopServer(t, func(w http.ResponseWriter, r *http.Request) {
		method, path = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})
	t.Cleanup(srv.Close)

	client, err := NewApiClient(t.Context(), srv.URL, NewIntegrationCredentials("id", "secret"), srv.Client())
	require.NoError(t, err)
	require.NoError(t, client.ClearCache(t.Context()))
	assert.Equal(t, http.MethodDelete, method)
	assert.Equal(t, "/api/_action/cache", path)
}

func TestInfoDetectsCloudShop(t *testing.T) {
	isolateTokenCache(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"token","expires_in":3600}`))
		case "/api/_info/config":
			_, _ = w.Write([]byte(`{"version":"6.6.5.0","bundles":{"SaasRufus":{"js":[]}}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := NewApiClient(t.Context(), srv.URL, NewIntegrationCredentials("id", "secret"), srv.Client())
	require.NoError(t, err)

	info, err := client.Info(t.Context())
	require.NoError(t, err)
	assert.True(t, info.IsCloudShop())
	assert.False(t, InfoResponse{Bundles: map[string]infoResponseBundle{}}.IsCloudShop())
}

func TestListStorefrontSalesChannels(t *testing.T) {
	var gotBody map[string]any
	srv := newShopServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/search/sales-channel", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))
		_, _ = w.Write([]byte(`{"data":[{"id":"sc1","name":"Storefront","typeId":"8a243080f92e4c719546314b577cf82b","active":true,"domains":[{"id":"d1","url":"https://shop.test"}]}]}`))
	})
	t.Cleanup(srv.Close)

	client, err := NewApiClient(t.Context(), srv.URL, NewIntegrationCredentials("id", "secret"), srv.Client())
	require.NoError(t, err)

	channels, err := client.ListStorefrontSalesChannels(t.Context())
	require.NoError(t, err)
	require.Len(t, channels, 1)
	assert.Equal(t, "sc1", channels[0].Id)
	assert.Equal(t, "https://shop.test", channels[0].Domains[0].Url)

	filters, ok := gotBody["filter"].([]any)
	require.True(t, ok)
	require.Len(t, filters, 2)
	assert.Contains(t, gotBody, "associations")
}

func TestFindThemeForSalesChannel(t *testing.T) {
	srv := newShopServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/search/theme", r.URL.Path)
		_, _ = w.Write([]byte(`{"data":[{"id":"theme-1","name":"Default","technicalName":"Storefront"}]}`))
	})
	t.Cleanup(srv.Close)

	client, err := NewApiClient(t.Context(), srv.URL, NewIntegrationCredentials("id", "secret"), srv.Client())
	require.NoError(t, err)

	theme, err := client.FindThemeForSalesChannel(t.Context(), "sc1")
	require.NoError(t, err)
	assert.Equal(t, "theme-1", theme.Id)
}

func TestFindThemeForSalesChannelNotFound(t *testing.T) {
	srv := newShopServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	})
	t.Cleanup(srv.Close)

	client, err := NewApiClient(t.Context(), srv.URL, NewIntegrationCredentials("id", "secret"), srv.Client())
	require.NoError(t, err)

	_, err = client.FindThemeForSalesChannel(t.Context(), "missing")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestExtensionManagerListAvailable(t *testing.T) {
	srv := newShopServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/_action/extension/installed", r.URL.Path)
		_, _ = w.Write([]byte(`[{"name":"SwagPayPal","version":"1.0.0","latestVersion":"1.1.0","active":true,"type":"plugin"}]`))
	})
	t.Cleanup(srv.Close)

	client, err := NewApiClient(t.Context(), srv.URL, NewIntegrationCredentials("id", "secret"), srv.Client())
	require.NoError(t, err)

	list, err := client.ExtensionManager.ListAvailable(t.Context())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "SwagPayPal", list.GetByName("SwagPayPal").Name)
	assert.True(t, list[0].IsUpdatable())
	assert.True(t, list[0].IsPlugin())
}

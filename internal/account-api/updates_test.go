package account_api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBlocker(t *testing.T) {
	assert.False(t, (UpdateCheckExtensionCompatibilityStatus{Type: "success"}).IsBlocker())
	assert.False(t, (UpdateCheckExtensionCompatibilityStatus{Type: ""}).IsBlocker())
	assert.True(t, (UpdateCheckExtensionCompatibilityStatus{Type: "error"}).IsBlocker())
	assert.True(t, (UpdateCheckExtensionCompatibilityStatus{Type: "warning"}).IsBlocker())
}

func TestGetFutureExtensionUpdates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/swplatform/autoupdate", r.URL.Path)
		assert.Equal(t, "en-GB", r.URL.Query().Get("language"))
		assert.Equal(t, "6.5.0.0", r.URL.Query().Get("shopwareVersion"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "6.6.0.0", body["futureShopwareVersion"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]UpdateCheckExtensionCompatibility{
			{Name: "SwagPayPal", Status: UpdateCheckExtensionCompatibilityStatus{Type: "success", Name: "compatible"}},
		})
	}))
	defer srv.Close()

	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", srv.URL)

	results, err := GetFutureExtensionUpdates(t.Context(), "6.5.0.0", "6.6.0.0", []UpdateCheckExtension{
		{Name: "SwagPayPal", Version: "1.0.0"},
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "SwagPayPal", results[0].Name)
	assert.Equal(t, "success", results[0].Status.Type)
}

func TestGetFutureExtensionUpdatesNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", srv.URL)

	_, err := GetFutureExtensionUpdates(t.Context(), "6.5.0.0", "6.6.0.0", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API returned non-OK status: 500")
}

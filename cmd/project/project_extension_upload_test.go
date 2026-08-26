package project

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEmptyExtensionListShop fakes the Admin API surface of an empty shop and records requests as "METHOD path".
func newEmptyExtensionListShop(t *testing.T, failSecondList bool) (*httptest.Server, func() []string) {
	t.Helper()

	var mu sync.Mutex
	var calls []string
	listCalls := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.Path)
		isList := r.Method == http.MethodGet && r.URL.Path == "/api/_action/extension/installed"
		if isList {
			listCalls++
		}
		failList := isList && failSecondList && listCalls > 1
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer","expires_in":3600}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/_info/config":
			_, _ = w.Write([]byte(`{"version":"6.6.5.0","bundles":{}}`))
		case isList:
			if failList {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/_action/extension/upload":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/_action/extension/refresh":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), calls...)
	}
}

func setupUploadActivateEnv(t *testing.T, srvURL string) {
	t.Helper()

	clearShopClientEnv(t)
	t.Setenv("PROJECT_ROOT", t.TempDir())
	t.Setenv("SHOPWARE_CLI_API_URL", srvURL)
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "test")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "test")
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")

	require.NoError(t, projectExtensionUploadCmd.PersistentFlags().Set("activate", "true"))
	t.Cleanup(func() {
		require.NoError(t, projectExtensionUploadCmd.PersistentFlags().Set("activate", "false"))
		projectExtensionUploadCmd.PersistentFlags().Lookup("activate").Changed = false
	})
}

func TestExtensionUploadActivateFailsWhenShopDoesNotListExtension(t *testing.T) {
	srv, calls := newEmptyExtensionListShop(t, false)
	pluginDir := writeMinimalPlugin(t)
	setupUploadActivateEnv(t, srv.URL)

	projectExtensionUploadCmd.SetContext(t.Context())
	err := projectExtensionUploadCmd.RunE(projectExtensionUploadCmd, []string{pluginDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot run lifecycle events")
	assert.Contains(t, err.Error(), "FroshTest")

	// The installed list must be queried after the refresh call.
	got := calls()
	refreshIdx, lastListIdx := -1, -1
	for i, call := range got {
		switch call {
		case "POST /api/_action/extension/refresh":
			refreshIdx = i
		case "GET /api/_action/extension/installed":
			lastListIdx = i
		}
	}
	require.NotEqual(t, -1, refreshIdx)
	require.NotEqual(t, -1, lastListIdx)
	assert.Greater(t, lastListIdx, refreshIdx)
}

func TestExtensionUploadActivateSurfacesListFetchError(t *testing.T) {
	srv, _ := newEmptyExtensionListShop(t, true)
	pluginDir := writeMinimalPlugin(t)
	setupUploadActivateEnv(t, srv.URL)

	projectExtensionUploadCmd.SetContext(t.Context())
	err := projectExtensionUploadCmd.RunE(projectExtensionUploadCmd, []string{pluginDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "got http code 500")
	assert.NotContains(t, err.Error(), "cannot run lifecycle events")
}

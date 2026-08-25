package project

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newEmptyExtensionListShop fakes the minimal Admin API surface the upload
// command touches, always reporting an empty installed-extension list.
// Requests outside that surface get a 404 so they surface as command errors.
func newEmptyExtensionListShop(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/oauth/token":
			_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer","expires_in":3600}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/_info/config":
			_, _ = w.Write([]byte(`{"version":"6.6.5.0","bundles":{}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/_action/extension/installed":
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

	return srv
}

func TestExtensionUploadActivateFailsWhenShopDoesNotListExtension(t *testing.T) {
	srv := newEmptyExtensionListShop(t)
	pluginDir := writeMinimalPlugin(t)

	clearShopClientEnv(t)
	t.Setenv("PROJECT_ROOT", t.TempDir())
	t.Setenv("SHOPWARE_CLI_API_URL", srv.URL)
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "test")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "test")
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")

	require.NoError(t, projectExtensionUploadCmd.PersistentFlags().Set("activate", "true"))
	t.Cleanup(func() {
		require.NoError(t, projectExtensionUploadCmd.PersistentFlags().Set("activate", "false"))
		projectExtensionUploadCmd.PersistentFlags().Lookup("activate").Changed = false
	})

	projectExtensionUploadCmd.SetContext(t.Context())
	err := projectExtensionUploadCmd.RunE(projectExtensionUploadCmd, []string{pluginDir})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot run lifecycle events")
	assert.Contains(t, err.Error(), "FroshTest")
}

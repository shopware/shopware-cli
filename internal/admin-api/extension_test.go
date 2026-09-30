package admin_sdk

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lifecycleCall struct {
	name   string
	call   func(ExtensionManagerService, ApiContext) (*http.Response, error)
	method string
	path   string
}

func lifecycleCalls() []lifecycleCall {
	return []lifecycleCall{
		{"install extension", func(m ExtensionManagerService, ctx ApiContext) (*http.Response, error) {
			return m.InstallExtension(ctx, "plugin", "FroshTools")
		}, http.MethodPost, "/api/_action/extension/install/plugin/FroshTools"},
		{"uninstall extension", func(m ExtensionManagerService, ctx ApiContext) (*http.Response, error) {
			return m.UninstallExtension(ctx, "plugin", "FroshTools")
		}, http.MethodPost, "/api/_action/extension/uninstall/plugin/FroshTools"},
		{"update extension", func(m ExtensionManagerService, ctx ApiContext) (*http.Response, error) {
			return m.UpdateExtension(ctx, "plugin", "FroshTools")
		}, http.MethodPost, "/api/_action/extension/update/plugin/FroshTools"},
		{"download extension", func(m ExtensionManagerService, ctx ApiContext) (*http.Response, error) {
			return m.DownloadExtension(ctx, "FroshTools")
		}, http.MethodPost, "/api/_action/extension/download/FroshTools"},
		{"activate extension", func(m ExtensionManagerService, ctx ApiContext) (*http.Response, error) {
			return m.ActivateExtension(ctx, "plugin", "FroshTools")
		}, http.MethodPut, "/api/_action/extension/activate/plugin/FroshTools"},
		{"deactivate extension", func(m ExtensionManagerService, ctx ApiContext) (*http.Response, error) {
			return m.DeactivateExtension(ctx, "plugin", "FroshTools")
		}, http.MethodPut, "/api/_action/extension/deactivate/plugin/FroshTools"},
		{"remove extension", func(m ExtensionManagerService, ctx ApiContext) (*http.Response, error) {
			return m.RemoveExtension(ctx, "plugin", "FroshTools")
		}, http.MethodPost, "/api/_action/extension/remove/plugin/FroshTools"},
	}
}

func TestExtensionManagerLifecycleRequests(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	manager := ExtensionManagerService{Client: &Client{url: srv.URL, client: srv.Client(), ShopwareVersion: version.Must(version.NewVersion("6.6.10.2"))}}

	for _, tc := range lifecycleCalls() {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := tc.call(manager, NewApiContext(t.Context()))
			require.NoError(t, err)
			_ = resp.Body.Close()

			assert.Equal(t, tc.method, gotMethod)
			assert.Equal(t, tc.path, gotPath)
		})
	}

	t.Run("remove extension before 6.6.10.2 uses DELETE", func(t *testing.T) {
		manager.Client.ShopwareVersion = version.Must(version.NewVersion("6.6.10.1"))

		resp, err := manager.RemoveExtension(NewApiContext(t.Context()), "plugin", "FroshTools")
		require.NoError(t, err)
		_ = resp.Body.Close()

		assert.Equal(t, http.MethodDelete, gotMethod)
	})
}

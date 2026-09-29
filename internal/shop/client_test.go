package shop

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/oauth/token" {
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "token",
				"expires_in":   3600,
			})
			if err != nil {
				t.Fatal(err)
			}

			return
		}

		if r.URL.Path == "/api/_info/config" {
			w.Header().Set("Content-Type", "application/json")
			err := json.NewEncoder(w).Encode(map[string]interface{}{
				"version": "6.5.0.0",
			})
			if err != nil {
				t.Fatal(err)
			}

			return
		}

		t.Fatalf("unhandled request to %s", r.URL.Path)
	}))
}

func Test_newShopCredentials_envIntegration(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "id")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "secret")

	cfg := &Config{}
	creds, err := newShopCredentials(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
}

func Test_newShopCredentials_envPassword(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_API_USERNAME", "user")
	t.Setenv("SHOPWARE_CLI_API_PASSWORD", "pass")

	cfg := &Config{}
	creds, err := newShopCredentials(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
}

func Test_newShopCredentials_configPassword(t *testing.T) {
	// No env vars needed for this test, but ensure clean state
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "")
	t.Setenv("SHOPWARE_CLI_API_USERNAME", "")
	t.Setenv("SHOPWARE_CLI_API_PASSWORD", "")

	cfg := &Config{
		AdminApi: &ConfigAdminApi{
			Username: "user",
			Password: "pass",
		},
	}
	creds, err := newShopCredentials(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
}

func Test_newShopCredentials_configIntegration(t *testing.T) {
	// No env vars needed for this test, but ensure clean state
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "")
	t.Setenv("SHOPWARE_CLI_API_USERNAME", "")
	t.Setenv("SHOPWARE_CLI_API_PASSWORD", "")

	cfg := &Config{
		AdminApi: &ConfigAdminApi{
			ClientId:     "id",
			ClientSecret: "secret",
		},
	}
	creds, err := newShopCredentials(cfg)
	assert.NoError(t, err)
	assert.NotNil(t, creds)
}

func Test_newShopCredentials_noConfig(t *testing.T) {
	// Ensure clean env state
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "")
	t.Setenv("SHOPWARE_CLI_API_USERNAME", "")
	t.Setenv("SHOPWARE_CLI_API_PASSWORD", "")

	cfg := &Config{}
	_, err := newShopCredentials(cfg)
	assert.Error(t, err)
}

func Test_NewShopClient(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	t.Setenv("SHOPWARE_CLI_API_URL", server.URL)
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "id")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "secret")

	cfg := &Config{}
	client, err := NewShopClient(t.Context(), cfg)
	assert.NoError(t, err)
	assert.NotNil(t, client)
}

func Test_NewShopClient_configUrl(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	// Ensure env URL is not set
	t.Setenv("SHOPWARE_CLI_API_URL", "")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "id")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "secret")

	cfg := &Config{URL: server.URL}
	client, err := NewShopClient(t.Context(), cfg)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	// Ideally, we'd check the client's configured URL here, but the SDK doesn't expose it easily.
}

func Test_NewShopClient_skipSSLCheck_env(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	t.Setenv("SHOPWARE_CLI_API_URL", server.URL)
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "id")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "secret")
	t.Setenv("SHOPWARE_CLI_API_DISABLE_SSL_CHECK", "true")

	cfg := &Config{}
	client, err := NewShopClient(t.Context(), cfg)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	// We cannot easily assert the TLS config here without reflection or modifying the original code.
}

func Test_NewShopClient_skipSSLCheck_config(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	t.Setenv("SHOPWARE_CLI_API_URL", server.URL)
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "id")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "secret")
	// Ensure env var is not set or false
	t.Setenv("SHOPWARE_CLI_API_DISABLE_SSL_CHECK", "false")

	cfg := &Config{
		AdminApi: &ConfigAdminApi{
			DisableSSLCheck: true,
		},
	}
	client, err := NewShopClient(t.Context(), cfg)
	assert.NoError(t, err)
	assert.NotNil(t, client)
	// We cannot easily assert the TLS config here without reflection or modifying the original code.
}

func Test_NewShopClient_NoURL(t *testing.T) {
	// Ensure env URL is not set
	t.Setenv("SHOPWARE_CLI_API_URL", "")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "id")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "secret")

	// Config with empty URL
	cfg := &Config{URL: ""}
	_, err := NewShopClient(t.Context(), cfg)
	// The current implementation doesn't check for empty URL
	// The error would come from the SDK later when it tries to make a request
	assert.Error(t, err)
}

func Test_NewShopClient_CredentialsError(t *testing.T) {
	server := newTestServer(t)
	defer server.Close()

	// Ensure clean env state
	t.Setenv("SHOPWARE_CLI_API_URL", server.URL)
	t.Setenv("SHOPWARE_CLI_API_CLIENT_ID", "")
	t.Setenv("SHOPWARE_CLI_API_CLIENT_SECRET", "")
	t.Setenv("SHOPWARE_CLI_API_USERNAME", "")
	t.Setenv("SHOPWARE_CLI_API_PASSWORD", "")

	// Config without credentials
	cfg := &Config{}
	_, err := NewShopClient(t.Context(), cfg)
	assert.Error(t, err)
}

func TestHasAdminAPICredentials(t *testing.T) {
	envKeys := []string{"SHOPWARE_CLI_API_CLIENT_ID", "SHOPWARE_CLI_API_CLIENT_SECRET", "SHOPWARE_CLI_API_USERNAME", "SHOPWARE_CLI_API_PASSWORD"}

	cases := []struct {
		name   string
		env    map[string]string
		config *Config
		want   bool
	}{
		{name: "integration env pair", env: map[string]string{"SHOPWARE_CLI_API_CLIENT_ID": "id", "SHOPWARE_CLI_API_CLIENT_SECRET": "secret"}, config: &Config{}, want: true},
		{name: "password env pair", env: map[string]string{"SHOPWARE_CLI_API_USERNAME": "user", "SHOPWARE_CLI_API_PASSWORD": "pass"}, config: &Config{}, want: true},
		{name: "env pair with nil config", env: map[string]string{"SHOPWARE_CLI_API_CLIENT_ID": "id", "SHOPWARE_CLI_API_CLIENT_SECRET": "secret"}, config: nil, want: true},
		{name: "half env pair", env: map[string]string{"SHOPWARE_CLI_API_CLIENT_ID": "id"}, config: &Config{}, want: false},
		{name: "config only", config: &Config{AdminApi: &ConfigAdminApi{ClientId: "id", ClientSecret: "secret"}}, want: true},
		{name: "config with empty fields", config: &Config{AdminApi: &ConfigAdminApi{}}, want: false},
		{name: "nothing", config: &Config{}, want: false},
		{name: "nil config", config: nil, want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, key := range envKeys {
				t.Setenv(key, tc.env[key])
			}

			assert.Equal(t, tc.want, HasAdminAPICredentials(tc.config))
		})
	}
}

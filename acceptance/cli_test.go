package acceptance

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/shyim/go-composer/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOfflineTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.Path)
	}))
	t.Cleanup(server.Close)

	refused := filepath.Join(t.TempDir(), "refused-requests")
	client := &http.Client{Transport: newOfflineTransport(server.URL, refused, http.DefaultTransport)}

	get := func(t *testing.T, rawURL string) (string, error) {
		t.Helper()

		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, rawURL, http.NoBody)
		require.NoError(t, err)

		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer func() { _ = resp.Body.Close() }()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		return string(body), nil
	}

	t.Run("packagist is served by the fake", func(t *testing.T) {
		body, err := get(t, repository.PackagistURL+"/p2/shopware/core.json")
		require.NoError(t, err)
		assert.Equal(t, "/p2/shopware/core.json", body)
	})

	t.Run("static data is served by the fake", func(t *testing.T) {
		body, err := get(t, "https://"+staticDataHost+phpVersionPath)
		require.NoError(t, err)
		assert.Equal(t, phpVersionPath, body)
	})

	t.Run("loopback passes through", func(t *testing.T) {
		body, err := get(t, server.URL+"/local")
		require.NoError(t, err)
		assert.Equal(t, "/local", body)
	})

	t.Run("other hosts are refused and recorded", func(t *testing.T) {
		_, err := get(t, "https://example.com/data.json")
		require.Error(t, err)

		requests, err := os.ReadFile(refused)
		require.NoError(t, err)
		assert.Equal(t, "GET https://example.com/data.json\n", string(requests))
	})
}

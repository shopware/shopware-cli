package project

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The existing image-proxy tests exercise a replica of the handler stack;
// these cover the real RunE's fail-fast validations, which all return before
// any server starts.
func TestImageProxyRunEValidatesUpstreamAndPublicDir(t *testing.T) {
	oldURL, oldConfigPath := imageProxyURL, projectConfigPath
	t.Cleanup(func() {
		imageProxyURL = oldURL
		projectConfigPath = oldConfigPath
	})

	run := func(t *testing.T, root string) error {
		t.Helper()
		t.Setenv("PROJECT_ROOT", root)
		projectImageProxyCmd.SetContext(t.Context())
		return projectImageProxyCmd.RunE(projectImageProxyCmd, nil)
	}

	t.Run("missing upstream", func(t *testing.T) {
		root := t.TempDir()
		projectConfigPath = filepath.Join(root, ".shopware-project.yml")
		imageProxyURL = ""

		err := run(t, root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "upstream URL must be provided")
	})

	t.Run("missing public dir", func(t *testing.T) {
		root := t.TempDir()
		projectConfigPath = filepath.Join(root, ".shopware-project.yml")
		imageProxyURL = "https://upstream.example.com"

		err := run(t, root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "public folder not found")
	})

	// The config fallback is consulted when the flag is absent: it passes the
	// upstream check and still fails at the public-dir check, proving order.
	t.Run("config fallback provides the upstream", func(t *testing.T) {
		root := t.TempDir()
		configPath := filepath.Join(root, ".shopware-project.yml")
		require.NoError(t, os.WriteFile(configPath,
			[]byte("compatibility_date: 2026-03-01\nimage_proxy:\n  url: https://upstream.example.com\n"), 0o644))
		projectConfigPath = configPath
		imageProxyURL = ""

		err := run(t, root)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "public folder not found")
	})
}

// waitUntilServing polls url until the proxy answers with 200.
func waitUntilServing(t *testing.T, url string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
		require.NoError(t, err)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			require.NoError(t, resp.Body.Close())
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("server did not start serving %s", url)
}

func httpGet(t *testing.T, url string) (string, http.Header) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	require.Equal(t, http.StatusOK, resp.StatusCode)
	return string(body), resp.Header
}

// waitForFile polls for path; the cache write happens when the proxy
// finishes streaming a response, slightly after the client sees the body.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("file %s did not appear", path)
}

func TestImageProxyServesCachesAndCleansUp(t *testing.T) {
	var upstreamHits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	}))
	t.Cleanup(upstream.Close)

	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "public"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "public", "local.txt"), []byte("local-file"), 0o644))
	t.Setenv("PROJECT_ROOT", root)

	l, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	require.NoError(t, l.Close())

	oldURL, oldPort, oldConfigPath := imageProxyURL, imageProxyPort, projectConfigPath
	t.Cleanup(func() { imageProxyURL, imageProxyPort, projectConfigPath = oldURL, oldPort, oldConfigPath })
	imageProxyURL, imageProxyPort = upstream.URL, port
	projectConfigPath = filepath.Join(root, ".shopware-project.yml")

	// Cancelling the context is the command's shutdown path, the same one
	// Ctrl+C takes through signal.NotifyContext in root.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	projectImageProxyCmd.SetContext(ctx)
	done := make(chan error, 1)
	go func() { done <- projectImageProxyCmd.RunE(projectImageProxyCmd, nil) }()

	base := "http://127.0.0.1:" + port
	waitUntilServing(t, base+"/local.txt")

	// The Shopware filesystem override exists while the proxy runs.
	configFile := filepath.Join(root, "config", "packages", "zzz-sw-cli-image-proxy.yml")
	content, err := os.ReadFile(configFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "http://localhost:"+port)

	// Local public files win; upstream is never asked.
	body, _ := httpGet(t, base+"/local.txt")
	assert.Equal(t, "local-file", body)
	assert.EqualValues(t, 0, upstreamHits.Load())

	// An unknown path proxies to upstream and lands in the cache.
	body, _ = httpGet(t, base+"/media/img.png")
	assert.Equal(t, "png-bytes", body)
	assert.EqualValues(t, 1, upstreamHits.Load())
	waitForFile(t, filepath.Join(root, "var", "cache", "image-proxy", "_media_img.png"))

	// The second request is a cache hit with the stored content type.
	body, header := httpGet(t, base+"/media/img.png")
	assert.Equal(t, "png-bytes", body)
	assert.Equal(t, "HIT", header.Get("X-Cache"))
	assert.Equal(t, "image/png", header.Get("Content-Type"))
	assert.EqualValues(t, 1, upstreamHits.Load())

	// Shutdown removes the override, the guard against permanently
	// rewriting a real shop's media URL.
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("image proxy did not shut down after context cancellation")
	}
	assert.NoFileExists(t, configFile)
}

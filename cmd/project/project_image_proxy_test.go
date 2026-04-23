package project

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NYTimes/gziphandler"
	"github.com/stretchr/testify/assert"
)

func TestImageProxySVG(t *testing.T) {
	svgContent := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">` + strings.Repeat(`<circle cx="50" cy="50" r="40" fill="red"/>`, 100) + `</svg>`

	gzipWrapper, _ := gziphandler.GzipHandlerWithOpts(
		gziphandler.ContentTypes([]string{
			"text/html", "text/css", "text/javascript",
			"application/javascript", "application/json",
			"image/svg+xml", "text/xml", "application/xml",
		}),
	)

	// Helper: create a proxy handler with caching via ModifyResponse (matching real implementation)
	makeProxyStack := func(t *testing.T, upstreamURL *url.URL) http.Handler {
		t.Helper()

		cacheDir := t.TempDir()

		proxy := httputil.NewSingleHostReverseProxy(upstreamURL)

		defaultDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			defaultDirector(req)
			req.Header.Del("Accept-Encoding")
		}

		proxy.ModifyResponse = func(res *http.Response) error {
			if res.StatusCode != http.StatusOK {
				return nil
			}

			cleanPath := filepath.Clean(res.Request.URL.Path)
			contentType := res.Header.Get("Content-Type")
			cachePath := filepath.Join(cacheDir, strings.ReplaceAll(cleanPath, "/", "_"))
			cacheMetaPath := cachePath + ".meta"

			var buf bytes.Buffer
			res.Body = &cacheReadCloser{
				ReadCloser: res.Body,
				tee:        io.TeeReader(res.Body, &buf),
				onClose: func() {
					if buf.Len() == 0 {
						return
					}
					_ = os.WriteFile(cachePath, buf.Bytes(), 0644)
					if contentType != "" {
						_ = os.WriteFile(cacheMetaPath, []byte(contentType), 0644)
					}
				},
			}

			return nil
		}

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cleanPath := filepath.Clean(r.URL.Path)

			// Check cache
			cachePath := filepath.Join(cacheDir, strings.ReplaceAll(cleanPath, "/", "_"))
			cacheMetaPath := cachePath + ".meta"
			if data, err := os.ReadFile(cachePath); err == nil {
				if metaData, err := os.ReadFile(cacheMetaPath); err == nil {
					w.Header().Set("Content-Type", string(metaData))
				}
				w.Header().Set("X-Cache", "HIT")
				_, _ = w.Write(data)
				return
			}

			r.URL.Host = upstreamURL.Host
			r.URL.Scheme = upstreamURL.Scheme
			r.Host = upstreamURL.Host

			proxy.ServeHTTP(w, r)
		})

		return gzipWrapper(handler)
	}

	t.Run("svg with gzip accept", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(svgContent))
		}))
		defer upstream.Close()

		upstreamURL, _ := url.Parse(upstream.URL)
		server := httptest.NewServer(makeProxyStack(t, upstreamURL))
		defer server.Close()

		client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
		req, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/test.svg", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		resp, err := client.Do(req)
		assert.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, len(body) > 0, "body should not be empty")
		assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

		gr, err := gzip.NewReader(bytes.NewReader(body))
		assert.NoError(t, err)
		decompressed, _ := io.ReadAll(gr)
		assert.Equal(t, svgContent, string(decompressed))
	})

	t.Run("svg without gzip accept", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(svgContent))
		}))
		defer upstream.Close()

		upstreamURL, _ := url.Parse(upstream.URL)
		server := httptest.NewServer(makeProxyStack(t, upstreamURL))
		defer server.Close()

		client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
		req, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/test.svg", nil)
		resp, err := client.Do(req)
		assert.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, svgContent, string(body))
	})

	t.Run("svg default go client", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(svgContent))
		}))
		defer upstream.Close()

		upstreamURL, _ := url.Parse(upstream.URL)
		server := httptest.NewServer(makeProxyStack(t, upstreamURL))
		defer server.Close()

		req, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/test.svg", nil)
		resp, err := http.DefaultClient.Do(req)
		assert.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, svgContent, string(body))
	})

	t.Run("svg cache hit", func(t *testing.T) {
		requestCount := 0
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestCount++
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(svgContent))
		}))
		defer upstream.Close()

		upstreamURL, _ := url.Parse(upstream.URL)
		server := httptest.NewServer(makeProxyStack(t, upstreamURL))
		defer server.Close()

		client := &http.Client{Transport: &http.Transport{DisableCompression: true}}

		// First request: cache miss
		req, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/test.svg", nil)
		resp, err := client.Do(req)
		assert.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		assert.Equal(t, svgContent, string(body))
		assert.Equal(t, 1, requestCount)

		// Second request: cache hit
		req2, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/test.svg", nil)
		resp2, err := client.Do(req2)
		assert.NoError(t, err)
		body2, _ := io.ReadAll(resp2.Body)
		_ = resp2.Body.Close()
		assert.Equal(t, svgContent, string(body2))
		assert.Equal(t, "HIT", resp2.Header.Get("X-Cache"))
		assert.Equal(t, 1, requestCount) // upstream was not hit again
	})

	t.Run("png not gzip compressed", func(t *testing.T) {
		pngData := bytes.Repeat([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, 250)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngData)
		}))
		defer upstream.Close()

		upstreamURL, _ := url.Parse(upstream.URL)
		server := httptest.NewServer(makeProxyStack(t, upstreamURL))
		defer server.Close()

		client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
		req, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/test.png", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		resp, err := client.Do(req)
		assert.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// PNG should NOT be gzip compressed since we removed it from content types
		assert.Empty(t, resp.Header.Get("Content-Encoding"))
		assert.Equal(t, pngData, body)
	})

	t.Run("chunked svg", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/svg+xml")
			// Write in chunks without Content-Length
			parts := strings.SplitAfter(svgContent, "/>")
			for _, part := range parts {
				_, _ = w.Write([]byte(part))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		}))
		defer upstream.Close()

		upstreamURL, _ := url.Parse(upstream.URL)
		server := httptest.NewServer(makeProxyStack(t, upstreamURL))
		defer server.Close()

		client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
		req, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/test.svg", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		resp, err := client.Do(req)
		assert.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, len(body) > 0, "body should not be empty")

		if resp.Header.Get("Content-Encoding") == "gzip" {
			gr, err := gzip.NewReader(bytes.NewReader(body))
			assert.NoError(t, err)
			decompressed, _ := io.ReadAll(gr)
			assert.Equal(t, svgContent, string(decompressed))
		} else {
			assert.Equal(t, svgContent, string(body))
		}
	})

	t.Run("small svg below gzip minSize", func(t *testing.T) {
		smallSVG := `<svg xmlns="http://www.w3.org/2000/svg"><rect width="10" height="10"/></svg>`
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(smallSVG))
		}))
		defer upstream.Close()

		upstreamURL, _ := url.Parse(upstream.URL)
		server := httptest.NewServer(makeProxyStack(t, upstreamURL))
		defer server.Close()

		client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
		req, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/small.svg", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		resp, err := client.Do(req)
		assert.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// Small SVGs should not be compressed (below minSize)
		assert.Equal(t, smallSVG, string(body))
	})

	t.Run("browser-like request with brotli accept", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(svgContent))
		}))
		defer upstream.Close()

		upstreamURL, _ := url.Parse(upstream.URL)
		server := httptest.NewServer(makeProxyStack(t, upstreamURL))
		defer server.Close()

		client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
		req, _ := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/test.svg", nil)
		req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
		req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
		resp, err := client.Do(req)
		assert.NoError(t, err)
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.True(t, len(body) > 0)
		assert.Equal(t, "gzip", resp.Header.Get("Content-Encoding"))

		gr, err := gzip.NewReader(bytes.NewReader(body))
		assert.NoError(t, err)
		decompressed, _ := io.ReadAll(gr)
		assert.Equal(t, svgContent, string(decompressed))
	})
}

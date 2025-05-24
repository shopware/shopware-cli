package project

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/NYTimes/gziphandler"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
)

var (
	imageProxyPort        string
	imageProxyURL         string
	imageProxyClear       bool
	imageProxyExternalURL string
	imageProxySkipConfig  bool
)

type responseCapture struct {
	http.ResponseWriter
	body        *bytes.Buffer
	statusCode  *int
	contentType *string
}

func (r *responseCapture) WriteHeader(code int) {
	*r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseCapture) Write(b []byte) (int, error) {
	if *r.statusCode == 0 {
		*r.statusCode = http.StatusOK
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *responseCapture) Header() http.Header {
	h := r.ResponseWriter.Header()
	if ct := h.Get("Content-Type"); ct != "" {
		*r.contentType = ct
	}
	return h
}

var projectImageProxyCmd = &cobra.Command{
	Use:   "image-proxy",
	Short: "Start a proxy server for serving images from the public folder",
	Long: `Start an HTTP server that serves files from the public folder of the closest Shopware project.
If a file is not found locally, it proxies the request to the upstream server.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		cfg, err := shop.ReadConfig(projectConfigPath, true)
		if err != nil {
			return err
		}

		// Determine upstream URL
		upstreamURL := imageProxyURL
		if upstreamURL == "" && cfg.ImageProxy != nil && cfg.ImageProxy.URL != "" {
			upstreamURL = cfg.ImageProxy.URL
		}

		if upstreamURL == "" {
			return fmt.Errorf("upstream URL must be provided either via --url flag or in .shopware-project.yml")
		}

		// Parse upstream URL
		upstream, err := url.Parse(upstreamURL)
		if err != nil {
			return fmt.Errorf("invalid upstream URL: %w", err)
		}

		// Determine public folder path
		publicPath := filepath.Join(path, "public")
		stat, err := os.Stat(publicPath)
		if err != nil || !stat.IsDir() {
			return fmt.Errorf("public folder not found at %s", publicPath)
		}

		// Create cache directory
		cacheDir := filepath.Join(path, "var", "cache", "image-proxy")

		// Clear cache if requested
		if imageProxyClear {
			logging.FromContext(cmd.Context()).Infof("Clearing cache directory: %s", cacheDir)
			_ = os.RemoveAll(cacheDir)
		}

		// Ensure cache directory exists
		if err := os.MkdirAll(cacheDir, 0755); err != nil {
			return fmt.Errorf("failed to create cache directory: %w", err)
		}

		// Create reverse proxy with custom transport to capture responses
		proxy := httputil.NewSingleHostReverseProxy(upstream)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			logging.FromContext(cmd.Context()).Errorf("proxy error: %v", err)
			http.Error(w, "Bad Gateway", http.StatusBadGateway)
		}

		// Create handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Clean the path
			cleanPath := filepath.Clean(r.URL.Path)
			if cleanPath == "/" {
				cleanPath = "/index.html"
			}

			// Try to serve from local public folder
			localPath := filepath.Join(publicPath, cleanPath)

			// Security check: ensure the path is within public folder
			if !strings.HasPrefix(localPath, publicPath) {
				http.Error(w, "Invalid path", http.StatusBadRequest)
				return
			}

			// Check if file exists locally
			if info, err := os.Stat(localPath); err == nil && !info.IsDir() {
				logging.FromContext(cmd.Context()).Debugf("Serving local file: %s", cleanPath)
				http.ServeFile(w, r, localPath)
				return
			}

			// Check cache
			cachePath := filepath.Join(cacheDir, strings.ReplaceAll(cleanPath, "/", "_"))
			cacheMetaPath := cachePath + ".meta"
			if data, err := os.ReadFile(cachePath); err == nil {
				logging.FromContext(cmd.Context()).Debugf("Serving from cache: %s", cleanPath)

				// Read content type from meta file if it exists
				if metaData, err := os.ReadFile(cacheMetaPath); err == nil {
					w.Header().Set("Content-Type", string(metaData))
				}

				w.Header().Set("X-Cache", "HIT")
				_, _ = w.Write(data)
				return
			}

			// If not found locally or in cache, proxy to upstream
			logging.FromContext(cmd.Context()).Debugf("Proxying to upstream: %s", cleanPath)

			// Create a custom response writer to capture the response
			var buf bytes.Buffer
			var statusCode int
			var contentType string
			captureWriter := &responseCapture{
				ResponseWriter: w,
				body:           &buf,
				statusCode:     &statusCode,
				contentType:    &contentType,
			}

			// Preserve the original path in the proxied request
			r.URL.Host = upstream.Host
			r.URL.Scheme = upstream.Scheme
			r.Host = upstream.Host

			proxy.ServeHTTP(captureWriter, r)

			// Cache successful responses
			if statusCode == http.StatusOK && buf.Len() > 0 {
				cachePath := filepath.Join(cacheDir, strings.ReplaceAll(cleanPath, "/", "_"))
				cacheMetaPath := cachePath + ".meta"

				// Ensure cache subdirectories exist
				if dir := filepath.Dir(cachePath); dir != cacheDir {
					_ = os.MkdirAll(dir, 0755)
				}

				// Write the file content
				if err := os.WriteFile(cachePath, buf.Bytes(), 0644); err == nil {
					// Write content type to meta file
					if contentType != "" {
						_ = os.WriteFile(cacheMetaPath, []byte(contentType), 0644)
					}
					logging.FromContext(cmd.Context()).Debugf("Cached file: %s", cleanPath)
				}
			}
		})

		// Prepare server address
		addr := fmt.Sprintf(":%s", imageProxyPort)

		// Setup config file management if not skipped
		var cleanup func()
		if !imageProxySkipConfig {
			// Create Shopware config file
			configDir := filepath.Join(path, "config", "packages")
			configFile := filepath.Join(configDir, "zzz-sw-cli-image-proxy.yml")

			// Ensure config directory exists
			if err := os.MkdirAll(configDir, 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			// Determine the URL to use in Shopware config
			configURL := fmt.Sprintf("http://localhost:%s", imageProxyPort)
			if imageProxyExternalURL != "" {
				configURL = strings.TrimSuffix(imageProxyExternalURL, "/")
			}

			// Write Shopware configuration
			configContent := fmt.Sprintf(`shopware:
  filesystem:
    public:
      type: "local"
      url: '%s'
      config:
        root: "%%kernel.project_dir%%/public"
`, configURL)

			if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
				return fmt.Errorf("failed to write Shopware config: %w", err)
			}

			logging.FromContext(cmd.Context()).Infof("Created Shopware config: %s (URL: %s)", configFile, configURL)

			// Setup cleanup handler
			cleanup = func() {
				if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
					logging.FromContext(cmd.Context()).Errorf("Failed to remove config file: %v", err)
				} else {
					logging.FromContext(cmd.Context()).Infof("Removed Shopware config: %s", configFile)
				}
			}

			// Handle interrupt signals
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			go func() {
				<-sigChan
				if cleanup != nil {
					cleanup()
				}
				os.Exit(0)
			}()

			// Ensure cleanup on normal exit
			defer cleanup()
		} else {
			logging.FromContext(cmd.Context()).Infof("Skipping Shopware config file creation")
		}

		// Start server
		logging.FromContext(cmd.Context()).Infof("Starting image proxy server on %s", addr)
		logging.FromContext(cmd.Context()).Infof("Serving files from: %s", publicPath)
		logging.FromContext(cmd.Context()).Infof("Proxying to: %s", upstreamURL)
		logging.FromContext(cmd.Context()).Infof("Cache directory: %s", cacheDir)

		// Enable gzip compression for common web content types
		gzipWrapper, _ := gziphandler.GzipHandlerWithOpts(
			gziphandler.ContentTypes([]string{
				"text/html",
				"text/css",
				"text/javascript",
				"application/javascript",
				"application/json",
				"application/vnd.api+json",
				"image/svg+xml",
				"image/jpeg",
				"image/png",
				"image/gif",
				"image/webp",
				"text/xml",
				"application/xml",
				"text/plain",
			}),
		)

		server := &http.Server{
			Addr:    addr,
			Handler: gzipWrapper(handler),
		}

		return server.ListenAndServe()
	},
}

func init() {
	projectRootCmd.AddCommand(projectImageProxyCmd)
	projectImageProxyCmd.Flags().StringVar(&imageProxyPort, "port", "8080", "Port to listen on")
	projectImageProxyCmd.Flags().StringVar(&imageProxyURL, "url", "", "Upstream server URL (overrides config)")
	projectImageProxyCmd.Flags().BoolVar(&imageProxyClear, "clear", false, "Clear cache before starting")
	projectImageProxyCmd.Flags().StringVar(&imageProxyExternalURL, "external-url", "", "External URL for Shopware config (e.g., for reverse proxy setups)")
	projectImageProxyCmd.Flags().BoolVar(&imageProxySkipConfig, "skip-config", false, "Skip creating Shopware config file")
}

package phplint

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/logging"
)

func findPHPWasmFile(ctx context.Context, phpVersion string) ([]byte, error) {
	expectedFile := "php-" + phpVersion + ".wasm"
	cacheKey := "wasm/php/" + expectedFile

	// Create cache instance for PHP WASM files
	cache := system.GetCacheWithPrefix("php-wasm")

	// Try to get from cache first
	reader, err := cache.Get(ctx, cacheKey)
	if err == nil {
		defer func() {
			if reader != nil {
				_ = reader.Close()
			}
		}()
		return io.ReadAll(reader)
	}

	// If not found in cache (err == system.ErrCacheNotFound), download it
	if err != system.ErrCacheNotFound {
		return nil, fmt.Errorf("cache error: %w", err)
	}

	downloadUrl := "https://github.com/shopwareLabs/php-cli-wasm-binaries/releases/download/1.0.0/" + expectedFile

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadUrl, nil)
	if err != nil {
		return nil, err
	}

	r.Header.Set("accept", "application/octet-stream")

	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("cannot download php-wasm binary: %s (%s)", resp.Status, downloadUrl)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("findPHPWasmFile: %v", err)
	}

	// Store in cache for future use
	if err := cache.Set(ctx, cacheKey, bytes.NewReader(data)); err != nil {
		logging.FromContext(ctx).Debugf("cannot cache php-wasm binary: %v", err)
	}

	return data, nil
}

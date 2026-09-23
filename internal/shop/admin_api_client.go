package shop

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/shopware/shopware-cli/internal/system"
	shopware "github.com/shopwareLabs/go-shopware-http-client"
)

const (
	tokenStorageKey = "shopware-cli"
	userAgent       = "shopware-cli"
)

// tokenCacheDirEnv overrides the on-disk OAuth token cache directory.
// Useful for tests to keep token caching hermetic.
const tokenCacheDirEnv = "SHOPWARE_CLI_TOKEN_CACHE_DIR"

// newTokenStorage returns a shop-scoped token storage backed by the shared
// file cache, falling back to in-memory when the file backend is unavailable.
// See https://github.com/shopwareLabs/go-shopware-http-client/pull/3.
func newTokenStorage(shopURL string) shopware.TokenStorage {
	dir := os.Getenv(tokenCacheDirEnv)
	if dir == "" {
		dir = filepath.Join(system.GetShopwareCliCacheDir(), "admin-api-tokens")
	}

	store, err := shopware.NewFileTokenStorage(dir)
	if err != nil {
		return shopware.NewInMemoryTokenStorage()
	}

	return shopware.NewScopedTokenStorage(store, shopURL)
}

// NewApiClient authenticates against the shop and returns a ready-to-use client.
func NewApiClient(ctx context.Context, shopURL string, credentials shopware.Credentials, httpClient *http.Client) (*shopware.Client, error) {
	client := shopware.NewClient(shopware.Config{
		BaseURL:         shopURL,
		Credentials:     credentials,
		HTTPClient:      httpClient,
		TokenStorage:    newTokenStorage(shopURL),
		TokenStorageKey: tokenStorageKey,
		UserAgent:       userAgent,
	})

	if err := client.Authenticate(ctx); err != nil {
		return nil, err
	}

	if _, err := client.Version(ctx); err != nil {
		return nil, fmt.Errorf("failed to fetch info: %w", err)
	}

	return client, nil
}

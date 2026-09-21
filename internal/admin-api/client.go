package admin_sdk

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	shopware "github.com/shopwareLabs/go-shopware-http-client"
	"github.com/shopwareLabs/go-shopware-http-client/extension"
	"github.com/shyim/go-version"
)

const (
	tokenStorageKey = "shopware-cli"
	userAgent       = "shopware-cli"
)

// Credentials is the OAuth grant used to authenticate against a shop.
type Credentials = shopware.Credentials

// ExtensionList is the Admin API extension listing.
type ExtensionList = extension.List

// ExtensionDetail describes a single extension returned by the Admin API.
type ExtensionDetail = extension.Detail

// Client is the CLI Admin API client. It wraps
// github.com/shopwareLabs/go-shopware-http-client and adds shopware-cli
// helpers (token export, cache clear, sales-channel lookup).
type Client struct {
	*shopware.Client

	tokens   shopware.TokenStorage
	tokenKey string

	ShopwareVersion  *version.Version
	ExtensionManager *extension.Manager
}

// NewIntegrationCredentials authenticates as a Shopware integration
// (client_credentials grant).
func NewIntegrationCredentials(clientID, clientSecret string) Credentials {
	return shopware.NewIntegrationCredentials(clientID, clientSecret)
}

// NewPasswordCredentials authenticates as an admin user (password grant).
func NewPasswordCredentials(username, password string) Credentials {
	return shopware.NewPasswordCredentials(username, password)
}

// NewApiClient authenticates against the shop and returns a ready-to-use Client.
func NewApiClient(ctx context.Context, shopURL string, credentials Credentials, httpClient *http.Client) (*Client, error) {
	storage := shopware.NewInMemoryTokenStorage()

	raw := shopware.NewClient(shopware.Config{
		BaseURL:         shopURL,
		Credentials:     credentials,
		HTTPClient:      httpClient,
		TokenStorage:    storage,
		TokenStorageKey: tokenStorageKey,
		UserAgent:       userAgent,
	})

	if err := raw.Authenticate(ctx); err != nil {
		return nil, err
	}

	versionStr, err := raw.Version(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch info: %w", err)
	}

	client := &Client{
		Client:           raw,
		tokens:           storage,
		tokenKey:         tokenStorageKey,
		ExtensionManager: extension.NewManager(raw),
	}

	if versionStr != "" {
		client.ShopwareVersion, err = version.NewVersion(versionStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse shopware version %q: %w", versionStr, err)
		}
	}

	return client, nil
}

// AccessToken returns a still-valid OAuth access token, refreshing if needed.
func (c *Client) AccessToken(ctx context.Context) (string, error) {
	if err := c.Authenticate(ctx); err != nil {
		return "", err
	}

	token, _, err := c.tokens.Get(ctx, c.tokenKey)
	if err != nil {
		return "", err
	}
	if token == "" {
		return "", errors.New("admin api token is not available")
	}

	return token, nil
}

// ClearCache invalidates the shop HTTP cache via DELETE /api/_action/cache.
func (c *Client) ClearCache(ctx context.Context) error {
	_, err := c.Delete(ctx, "/_action/cache", nil)
	return err
}

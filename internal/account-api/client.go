package account_api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"

	"github.com/shopware/shopware-cli/logging"
)

var httpUserAgent = "shopware-cli/0.0.0"

func SetUserAgent(userAgent string) {
	httpUserAgent = userAgent
}

type Client struct {
	Token *oauth2.Token `json:"token"`
}

func (c *Client) NewAuthenticatedRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	logging.FromContext(ctx).Debugf("%s: %s", method, path)
	r, err := http.NewRequestWithContext(ctx, method, path, body)
	if err != nil {
		return nil, err
	}

	r.Header.Set("content-type", "application/json")
	r.Header.Set("accept", "application/json")
	c.Token.SetAuthHeader(r)
	r.Header.Set("user-agent", httpUserAgent)

	return r, nil
}

func (*Client) doRequest(request *http.Request) ([]byte, error) {
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		_ = resp.Body.Close()

		return nil, fmt.Errorf("doRequest: %v", err)
	}

	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("doRequest: %v", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf(string(data)+", got status code %d", resp.StatusCode)
	}

	return data, nil
}

func (c *Client) isTokenValid() bool {
	if c.Token == nil {
		return false
	}

	return time.Until(c.Token.Expiry) > 60
}

const CacheFileName = "shopware-api-oauth2-token.json"

func getApiTokenCacheFilePath() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	shopwareCacheDir := filepath.Join(cacheDir, "shopware-cli")
	return filepath.Join(shopwareCacheDir, CacheFileName), nil
}

func createApiFromTokenCache(ctx context.Context) (*Client, error) {
	tokenFilePath, err := getApiTokenCacheFilePath()
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(tokenFilePath); os.IsNotExist(err) {
		return nil, err
	}

	content, err := os.ReadFile(tokenFilePath)
	if err != nil {
		return nil, err
	}

	var client *Client
	err = json.Unmarshal(content, &client)
	if err != nil {
		return nil, err
	}

	logging.FromContext(ctx).Debugf("Using token cache from %s", tokenFilePath)

	if !client.isTokenValid() {
		return nil, fmt.Errorf("token is expired")
	}

	return client, nil
}

func saveApiTokenToTokenCache(client *Client) error {
	tokenFilePath, err := getApiTokenCacheFilePath()
	if err != nil {
		return err
	}

	content, err := json.Marshal(client)
	if err != nil {
		return err
	}

	tokenFileDirectory := filepath.Dir(tokenFilePath)
	if _, err := os.Stat(tokenFileDirectory); os.IsNotExist(err) {
		err := os.MkdirAll(tokenFileDirectory, 0o750)
		if err != nil {
			return err
		}
	}

	err = os.WriteFile(tokenFilePath, content, os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}

func InvalidateTokenCache() error {
	tokenFilePath, err := getApiTokenCacheFilePath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(tokenFilePath); os.IsNotExist(err) {
		return nil
	}

	return os.Remove(tokenFilePath)
}

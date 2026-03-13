package account_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/shopware/shopware-cli/logging"
)

const legacyApiUrl = "https://api.shopware.com"

func NewApi(ctx context.Context) (*Client, error) {
	client, _ := createApiFromTokenCache(ctx)

	if client != nil && client.isTokenValid() {
		return client, nil
	}

	// Try legacy username/password auth from environment variables
	email := os.Getenv("SHOPWARE_CLI_ACCOUNT_EMAIL")
	password := os.Getenv("SHOPWARE_CLI_ACCOUNT_PASSWORD")

	if email != "" && password != "" {
		return loginWithCredentials(ctx, email, password)
	}

	// Fall back to interactive OAuth2 login
	token, err := InteractiveLogin(ctx)
	if err != nil {
		return nil, fmt.Errorf("login: %v", err)
	}

	client = &Client{Token: token}

	if err := saveApiTokenToTokenCache(client); err != nil {
		logging.FromContext(ctx).Errorf(fmt.Sprintf("Cannot save token cache: %v", err))
	}

	return client, nil
}

func loginWithCredentials(ctx context.Context, email, password string) (*Client, error) {
	s, err := json.Marshal(loginRequest{Email: email, Password: password})
	if err != nil {
		return nil, fmt.Errorf("login: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, legacyApiUrl+"/accesstokens", bytes.NewBuffer(s))
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login: %v", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("Cannot close response body: %v", err)
		}
	}()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("login: %v", err)
	}

	if resp.StatusCode != 200 {
		logging.FromContext(ctx).Debugf("Login failed with response: %s", string(data))

		var apiErr struct {
			Detail string `json:"detail"`
		}
		if err := json.Unmarshal(data, &apiErr); err == nil && apiErr.Detail != "" {
			return nil, fmt.Errorf("login failed: %s", apiErr.Detail)
		}

		return nil, fmt.Errorf("login failed. Check your credentials")
	}

	var tokenResp legacyToken
	if err := json.Unmarshal(data, &tokenResp); err != nil {
		return nil, fmt.Errorf("login: %v", err)
	}

	client := &Client{
		LegacyToken: &tokenResp,
	}

	if err := saveApiTokenToTokenCache(client); err != nil {
		logging.FromContext(ctx).Errorf(fmt.Sprintf("Cannot save token cache: %v", err))
	}

	return client, nil
}

type legacyToken struct {
	Token       string      `json:"token"`
	Expire      tokenExpire `json:"expire"`
	LegacyLogin bool        `json:"legacyLogin"`
}

type tokenExpire struct {
	Date         string `json:"date"`
	TimezoneType int    `json:"timezone_type"`
	Timezone     string `json:"timezone"`
}

type loginRequest struct {
	Email    string `json:"shopwareId"`
	Password string `json:"password"`
}

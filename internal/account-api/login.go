package account_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/shopware/shopware-cli/logging"
)

func NewApi(ctx context.Context) (*Client, error) {
	client, _ := createApiFromTokenCache(ctx)

	if client != nil && client.isTokenValid() {
		return client, nil
	}

	// Try OAuth2 client credentials from environment variables (for CI/CD)
	clientID := os.Getenv("SHOPWARE_CLI_ACCOUNT_CLIENT_ID")
	clientSecret := os.Getenv("SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET")

	if clientID != "" || clientSecret != "" {
		if clientID == "" || clientSecret == "" {
			return nil, fmt.Errorf("both SHOPWARE_CLI_ACCOUNT_CLIENT_ID and SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET must be set")
		}
		return loginWithClientCredentials(ctx, clientID, clientSecret)
	}

	// Try legacy username/password auth from environment variables
	email := os.Getenv("SHOPWARE_CLI_ACCOUNT_EMAIL")
	password := os.Getenv("SHOPWARE_CLI_ACCOUNT_PASSWORD")

	if email != "" && password != "" {
		logging.FromContext(ctx).Warnf("authentification with username/password is deprecated and will be removed in future. Please switch to OAuth2 client credentials, see https://developer.shopware.com/docs/products/cli/shopware-account-commands/authentication.html")
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

func loginWithClientCredentials(ctx context.Context, clientID, clientSecret string) (*Client, error) {
	conf := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     fmt.Sprintf("%s/oauth2/token", getOIDCEndpoint()),
		Scopes:       []string{ClientCredentialsScopes},
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	token, err := conf.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("client credentials login: %w", err)
	}

	client := &Client{Token: token}

	if err := saveApiTokenToTokenCache(client); err != nil {
		logging.FromContext(ctx).Errorf("Cannot save token cache: %v", err)
	}

	return client, nil
}

func loginWithCredentials(ctx context.Context, email, password string) (*Client, error) {
	s, err := json.Marshal(loginRequest{Email: email, Password: password})
	if err != nil {
		return nil, fmt.Errorf("login: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, getApiUrl()+"/accesstokens", bytes.NewBuffer(s))
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

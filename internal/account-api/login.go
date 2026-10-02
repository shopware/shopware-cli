package account_api

import (
	"context"
	"errors"
	"fmt"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/shopware/shopware-cli/internal/system"
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
			return nil, errors.New("both SHOPWARE_CLI_ACCOUNT_CLIENT_ID and SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET must be set")
		}
		return loginWithClientCredentials(ctx, clientID, clientSecret)
	}

	if !system.IsInteractionEnabled(ctx) {
		return nil, errors.New("not logged in and interaction is disabled: run \"shopware-cli account login\" in a terminal, or set SHOPWARE_CLI_ACCOUNT_CLIENT_ID and SHOPWARE_CLI_ACCOUNT_CLIENT_SECRET")
	}

	// Fall back to interactive OAuth2 login
	token, err := InteractiveLogin(ctx)
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}

	client = &Client{Token: token}

	if err := saveApiTokenToTokenCache(client); err != nil {
		logging.FromContext(ctx).Errorf("Cannot save token cache: %v", err)
	}

	return client, nil
}

func loginWithClientCredentials(ctx context.Context, clientID, clientSecret string) (*Client, error) {
	conf := &clientcredentials.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     getOIDCEndpoint() + "/oauth2/token",
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

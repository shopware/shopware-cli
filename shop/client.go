package shop

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
)

func newShopCredentials(config *Config) (adminSdk.OAuthCredentials, error) {
	clientId, clientSecret := os.Getenv("SHOPWARE_CLI_API_CLIENT_ID"), os.Getenv("SHOPWARE_CLI_API_CLIENT_SECRET")

	if clientId != "" && clientSecret != "" {
		return adminSdk.NewIntegrationCredentials(clientId, clientSecret, []string{"write"}), nil
	}

	username, password := os.Getenv("SHOPWARE_CLI_API_USERNAME"), os.Getenv("SHOPWARE_CLI_API_PASSWORD")

	if username != "" && password != "" {
		return adminSdk.NewPasswordCredentials(username, password, []string{"write"}), nil
	}

	if config.AdminApi == nil {
		return nil, fmt.Errorf("admin-api is not enabled in config")
	}

	if config.AdminApi.Username != "" {
		return adminSdk.NewPasswordCredentials(config.AdminApi.Username, config.AdminApi.Password, []string{"write"}), nil
	}

	return adminSdk.NewIntegrationCredentials(config.AdminApi.ClientId, config.AdminApi.ClientSecret, []string{"write"}), nil
}

func NewShopClient(ctx context.Context, config *Config) (*adminSdk.Client, error) {
	skipSSLCert := false

	if config.AdminApi != nil {
		skipSSLCert = config.AdminApi.DisableSSLCheck
	}

	if os.Getenv("SHOPWARE_CLI_API_DISABLE_SSL_CHECK") == "true" {
		skipSSLCert = true
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: skipSSLCert, // nolint:gosec
		},
	}
	client := &http.Client{Transport: tr}

	shopUrl := os.Getenv("SHOPWARE_CLI_API_URL")

	if shopUrl == "" {
		shopUrl = config.URL
	}

	creds, err := newShopCredentials(config)
	if err != nil {
		return nil, fmt.Errorf("newShopCredentials: %v", err)
	}

	return adminSdk.NewApiClient(ctx, shopUrl, creds, client)
}

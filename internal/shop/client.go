package shop

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"os"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
)

// ErrNoAdminAPICredentials is returned when neither the project config nor the environment provides Admin API credentials.
var ErrNoAdminAPICredentials = errors.New("no Admin API credentials configured: set environments.<name>.admin_api in .shopware-project.yml or SHOPWARE_CLI_API_CLIENT_ID and SHOPWARE_CLI_API_CLIENT_SECRET")

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
		return nil, ErrNoAdminAPICredentials
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
		return nil, err
	}

	return adminSdk.NewApiClient(ctx, shopUrl, creds, client)
}

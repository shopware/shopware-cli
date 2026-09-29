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
var ErrNoAdminAPICredentials = errors.New("no Admin API credentials configured: set environments.<name>.admin_api in .config/shopware-project.yml or SHOPWARE_CLI_API_CLIENT_ID and SHOPWARE_CLI_API_CLIENT_SECRET")

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

// HasAdminAPICredentials reports whether the environment or the config provides Admin API credentials.
func HasAdminAPICredentials(config *Config) bool {
	if os.Getenv("SHOPWARE_CLI_API_CLIENT_ID") != "" && os.Getenv("SHOPWARE_CLI_API_CLIENT_SECRET") != "" {
		return true
	}

	if os.Getenv("SHOPWARE_CLI_API_USERNAME") != "" && os.Getenv("SHOPWARE_CLI_API_PASSWORD") != "" {
		return true
	}

	return config != nil && config.IsAdminAPIConfigured()
}

// AdminAPIURL returns the shop URL for Admin API requests, SHOPWARE_CLI_API_URL first.
func AdminAPIURL(config *Config) string {
	if url := os.Getenv("SHOPWARE_CLI_API_URL"); url != "" {
		return url
	}

	if config == nil {
		return ""
	}

	return config.URL
}

// AdminAPIDisableSSLCheck reports whether SHOPWARE_CLI_API_DISABLE_SSL_CHECK or the config disables certificate checks.
func AdminAPIDisableSSLCheck(config *Config) bool {
	if os.Getenv("SHOPWARE_CLI_API_DISABLE_SSL_CHECK") == "true" {
		return true
	}

	return config != nil && config.AdminApi != nil && config.AdminApi.DisableSSLCheck
}

func NewShopClient(ctx context.Context, config *Config) (*adminSdk.Client, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: AdminAPIDisableSSLCheck(config), // nolint:gosec
		},
	}
	client := &http.Client{Transport: tr}

	creds, err := newShopCredentials(config)
	if err != nil {
		return nil, err
	}

	return adminSdk.NewApiClient(ctx, AdminAPIURL(config), creds, client)
}

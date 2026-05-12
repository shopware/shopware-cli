package account_api

import "os"

const OIDCScopes = "openid offline_access email profile extension_management_read_write"

// ClientCredentialsScopes excludes openid as it is not allowed for client credentials grant.
const ClientCredentialsScopes = "extension_management_read_write"

func isStaging() bool {
	return os.Getenv("SHOPWARE_CLI_ACCOUNT_STAGING") != ""
}

func getOIDCEndpoint() string {
	if v := os.Getenv("SHOPWARE_CLI_OIDC_ENDPOINT"); v != "" {
		return v
	}
	if isStaging() {
		return "https://auth-api.shopware.in"
	}
	return "https://auth-api.shopware.com"
}

func getOIDCClientID() string {
	if isStaging() {
		return "def413d7-4c4e-439f-8b51-74c352436b2f"
	}
	return "069d0a55-5237-4706-a5c9-7cb1a45f1e81"
}

func getApiUrl() string {
	if isStaging() {
		return "https://next-api.shopware.com"
	}
	return "https://api.shopware.com"
}

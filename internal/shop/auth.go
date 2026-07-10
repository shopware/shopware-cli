package shop

import (
	"os"

	"github.com/shyim/go-composer"
)

// ReadComposerAuth reads a Composer auth.json (via go-composer), then merges
// Shopware-specific environment configuration on top: the COMPOSER_AUTH env var
// (handled by go-composer's MergeEnv) and the SHOPWARE_PACKAGES_TOKEN convenience
// variable, which registers a bearer token for packages.shopware.com.
//
// A missing auth.json is not an error; an empty, env-populated Auth is returned.
func ReadComposerAuth(authFile string) (*composer.Auth, error) {
	auth, err := composer.ReadAuth(authFile)
	if err != nil {
		return nil, err
	}

	if err := auth.MergeEnv(); err != nil {
		return nil, err
	}

	if token := os.Getenv("SHOPWARE_PACKAGES_TOKEN"); token != "" {
		if auth.BearerAuth == nil {
			auth.BearerAuth = map[string]string{}
		}
		auth.BearerAuth["packages.shopware.com"] = token
	}

	return auth, nil
}

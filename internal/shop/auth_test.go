package shop

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadComposerAuthEnv(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "auth.json")

	t.Run("missing file returns empty auth", func(t *testing.T) {
		auth, err := ReadComposerAuth(missing)
		require.NoError(t, err)
		assert.Empty(t, auth.BearerAuth)
	})

	t.Run("with SHOPWARE_PACKAGES_TOKEN", func(t *testing.T) {
		t.Setenv("SHOPWARE_PACKAGES_TOKEN", "my-token")

		auth, err := ReadComposerAuth(missing)
		require.NoError(t, err)
		assert.Equal(t, "my-token", auth.BearerAuth["packages.shopware.com"])
	})

	t.Run("with COMPOSER_AUTH", func(t *testing.T) {
		t.Setenv("COMPOSER_AUTH", `{
			"http-basic": {
				"example.com": {
					"username": "user",
					"password": "password"
				}
			},
			"bearer": {
				"example.com": "bearer-token"
			}
		}`)

		auth, err := ReadComposerAuth(missing)
		require.NoError(t, err)
		assert.Equal(t, "user", auth.HTTPBasicAuth["example.com"].Username)
		assert.Equal(t, "password", auth.HTTPBasicAuth["example.com"].Password)
		assert.Equal(t, "bearer-token", auth.BearerAuth["example.com"])
	})

	t.Run("SHOPWARE_PACKAGES_TOKEN wins over COMPOSER_AUTH", func(t *testing.T) {
		t.Setenv("SHOPWARE_PACKAGES_TOKEN", "my-token")
		t.Setenv("COMPOSER_AUTH", `{
			"bearer": {
				"packages.shopware.com": "composer-token"
			}
		}`)

		auth, err := ReadComposerAuth(missing)
		require.NoError(t, err)
		assert.Equal(t, "my-token", auth.BearerAuth["packages.shopware.com"])
	})
}

package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUpsertEnvVarCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.local")

	assert.NoError(t, UpsertEnvVar(path, "APP_URL", "https://my-shop.127.0.0.1.sslip.io"))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "APP_URL=https://my-shop.127.0.0.1.sslip.io\n", string(content))
}

func TestUpsertEnvVarReplacesExisting(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.local")

	assert.NoError(t, os.WriteFile(path, []byte("APP_ENV=dev\nAPP_URL=http://localhost:8000\nMAILER_DSN=null://null\n"), 0o644))

	assert.NoError(t, UpsertEnvVar(path, "APP_URL", "https://my-shop.127.0.0.1.sslip.io"))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "APP_ENV=dev\nAPP_URL=https://my-shop.127.0.0.1.sslip.io\nMAILER_DSN=null://null\n", string(content))
}

func TestUpsertEnvVarAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.local")

	assert.NoError(t, os.WriteFile(path, []byte("APP_ENV=dev\n"), 0o644))

	assert.NoError(t, UpsertEnvVar(path, "APP_URL", "https://my-shop.127.0.0.1.sslip.io"))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "APP_ENV=dev\nAPP_URL=https://my-shop.127.0.0.1.sslip.io\n", string(content))
}

func TestUpsertEnvVarDoesNotMatchPrefixKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env.local")

	assert.NoError(t, os.WriteFile(path, []byte("APP_URL_INTERNAL=http://web:8000\n"), 0o644))

	assert.NoError(t, UpsertEnvVar(path, "APP_URL", "https://my-shop.127.0.0.1.sslip.io"))

	content, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Equal(t, "APP_URL_INTERNAL=http://web:8000\nAPP_URL=https://my-shop.127.0.0.1.sslip.io\n", string(content))
}

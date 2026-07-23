package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertEnvVar(t *testing.T) {
	t.Parallel()

	t.Run("creates the file when missing", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), ".env.local")
		require.NoError(t, UpsertEnvVar(path, "APP_URL", "http://shop1.shopware.local"))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "APP_URL=http://shop1.shopware.local\n", string(content))
	})

	t.Run("replaces an existing assignment and keeps other lines", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), ".env.local")
		require.NoError(t, os.WriteFile(path, []byte("FOO=bar\nAPP_URL=http://127.0.0.1:8000\nBAZ=qux\n"), 0o644))

		require.NoError(t, UpsertEnvVar(path, "APP_URL", "http://shop1.shopware.local"))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "FOO=bar\nAPP_URL=http://shop1.shopware.local\nBAZ=qux\n", string(content))
	})

	t.Run("appends when the key is missing", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), ".env.local")
		require.NoError(t, os.WriteFile(path, []byte("FOO=bar\n"), 0o644))

		require.NoError(t, UpsertEnvVar(path, "APP_URL", "http://shop1.shopware.local"))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "FOO=bar\nAPP_URL=http://shop1.shopware.local\n", string(content))
	})

	t.Run("does not match keys that only share a prefix", func(t *testing.T) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), ".env.local")
		require.NoError(t, os.WriteFile(path, []byte("APP_URL_EXTERNAL=http://example.com\n"), 0o644))

		require.NoError(t, UpsertEnvVar(path, "APP_URL", "http://shop1.shopware.local"))

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, "APP_URL_EXTERNAL=http://example.com\nAPP_URL=http://shop1.shopware.local\n", string(content))
	})
}

func TestReadEnvVar(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), ".env.local")
	require.NoError(t, os.WriteFile(path, []byte("FOO=bar\nAPP_URL=http://127.0.0.1:8000\n"), 0o644))

	assert.Equal(t, "http://127.0.0.1:8000", ReadEnvVar(path, "APP_URL"))
	assert.Equal(t, "", ReadEnvVar(path, "MISSING"))
	assert.Equal(t, "", ReadEnvVar(filepath.Join(t.TempDir(), "nope"), "APP_URL"))
}

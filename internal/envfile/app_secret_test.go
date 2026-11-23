package envfile

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var hexSecret = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestEnsureAppSecret(t *testing.T) {
	t.Parallel()

	t.Run("creates the file when missing", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		generated, err := EnsureAppSecret(root)
		require.NoError(t, err)
		assert.True(t, generated)
		assert.Regexp(t, hexSecret, ReadEnvVar(filepath.Join(root, ".env.local"), appSecretEnvKey))
	})

	t.Run("fills an empty value and keeps other lines", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		path := filepath.Join(root, ".env.local")
		require.NoError(t, os.WriteFile(path, []byte("APP_ENV=dev\nAPP_SECRET=\nFOO=bar\n"), 0o644))

		generated, err := EnsureAppSecret(root)
		require.NoError(t, err)
		assert.True(t, generated)

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		secret := ExtractEnvVar(content, appSecretEnvKey)
		assert.Regexp(t, hexSecret, secret)
		assert.Equal(t, "APP_ENV=dev\nAPP_SECRET="+secret+"\nFOO=bar\n", string(content))
	})

	t.Run("fills a quoted empty value", func(t *testing.T) {
		t.Parallel()

		for _, quoted := range []string{`""`, `''`} {
			root := t.TempDir()
			path := filepath.Join(root, ".env.local")
			require.NoError(t, os.WriteFile(path, []byte("APP_SECRET="+quoted+"\n"), 0o644))

			generated, err := EnsureAppSecret(root)
			require.NoError(t, err)
			assert.True(t, generated)
			assert.Regexp(t, hexSecret, ReadEnvVar(path, appSecretEnvKey))
		}
	})

	t.Run("keeps an existing value", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		path := filepath.Join(root, ".env.local")
		original := "APP_ENV=dev\nAPP_SECRET=keep-me\n"
		require.NoError(t, os.WriteFile(path, []byte(original), 0o644))

		generated, err := EnsureAppSecret(root)
		require.NoError(t, err)
		assert.False(t, generated)

		content, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.Equal(t, original, string(content))
	})

	t.Run("returns an error when the file cannot be written", func(t *testing.T) {
		t.Parallel()

		_, err := EnsureAppSecret(filepath.Join(t.TempDir(), "missing"))
		assert.Error(t, err)
	})
}

func TestKeepAppSecretOutOfEnv(t *testing.T) {
	t.Parallel()

	t.Run("blanks the secret in .env and generates one in .env.local", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROJECT_NAME=shop\nAPP_SECRET=committed\n"), 0o644))

		require.NoError(t, KeepAppSecretOutOfEnv(root))

		env, err := os.ReadFile(filepath.Join(root, ".env"))
		require.NoError(t, err)
		assert.Equal(t, "COMPOSE_PROJECT_NAME=shop\nAPP_SECRET=\n", string(env))
		assert.Regexp(t, hexSecret, ReadEnvVar(filepath.Join(root, ".env.local"), appSecretEnvKey))
	})

	t.Run("leaves .env alone without a secret", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		original := "COMPOSE_PROJECT_NAME=shop\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte(original), 0o644))

		require.NoError(t, KeepAppSecretOutOfEnv(root))

		env, err := os.ReadFile(filepath.Join(root, ".env"))
		require.NoError(t, err)
		assert.Equal(t, original, string(env))
		assert.Regexp(t, hexSecret, ReadEnvVar(filepath.Join(root, ".env.local"), appSecretEnvKey))
	})
}

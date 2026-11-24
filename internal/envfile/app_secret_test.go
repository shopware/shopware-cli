package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMoveAppSecretToEnvLocal(t *testing.T) {
	t.Parallel()

	t.Run("moves the secret from .env to .env.local", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("COMPOSE_PROJECT_NAME=shop\nAPP_SECRET=recipe-secret\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env.local"), []byte("APP_ENV=dev\n"), 0o644))

		require.NoError(t, MoveAppSecretToEnvLocal(root))

		env, err := os.ReadFile(filepath.Join(root, ".env"))
		require.NoError(t, err)
		assert.Equal(t, "COMPOSE_PROJECT_NAME=shop\nAPP_SECRET=\n", string(env))

		envLocal, err := os.ReadFile(filepath.Join(root, ".env.local"))
		require.NoError(t, err)
		assert.Equal(t, "APP_ENV=dev\nAPP_SECRET=recipe-secret\n", string(envLocal))
	})

	t.Run("does nothing without a secret in .env", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		original := "COMPOSE_PROJECT_NAME=shop\n"
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte(original), 0o644))

		require.NoError(t, MoveAppSecretToEnvLocal(root))

		env, err := os.ReadFile(filepath.Join(root, ".env"))
		require.NoError(t, err)
		assert.Equal(t, original, string(env))
		assert.NoFileExists(t, filepath.Join(root, ".env.local"))
	})

	t.Run("returns an error when .env cannot be read", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(root, ".env"), 0o755))

		assert.Error(t, MoveAppSecretToEnvLocal(root))
	})

	t.Run("returns an error when .env.local cannot be written", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, ".env"), []byte("APP_SECRET=recipe-secret\n"), 0o644))
		require.NoError(t, os.Mkdir(filepath.Join(root, ".env.local"), 0o755))

		assert.Error(t, MoveAppSecretToEnvLocal(root))
	})
}

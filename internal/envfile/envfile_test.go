package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadAppEnv_NoFiles(t *testing.T) {
	tempDir := t.TempDir()

	value, err := ReadAppEnv(tempDir)
	require.NoError(t, err)
	assert.Equal(t, "", value)
}

func TestReadAppEnv_FromEnv(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".env"), []byte("APP_ENV=dev\n"), 0o644))

	value, err := ReadAppEnv(tempDir)
	require.NoError(t, err)
	assert.Equal(t, "dev", value)
}

func TestReadAppEnv_EnvLocalOverridesEnv(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".env"), []byte("APP_ENV=prod\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".env.local"), []byte("APP_ENV=dev\n"), 0o644))

	value, err := ReadAppEnv(tempDir)
	require.NoError(t, err)
	assert.Equal(t, "dev", value)
}

func TestWriteAppEnv_CreatesFile(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, WriteAppEnv(tempDir, "prod"))

	content, err := os.ReadFile(filepath.Join(tempDir, ".env.local"))
	require.NoError(t, err)
	assert.Equal(t, "APP_ENV=prod\n", string(content))
}

func TestWriteAppEnv_ReplacesExistingLine(t *testing.T) {
	tempDir := t.TempDir()
	envLocal := filepath.Join(tempDir, ".env.local")
	require.NoError(t, os.WriteFile(envLocal, []byte("APP_SECRET=secret\nAPP_ENV=dev\nDATABASE_URL=mysql://...\n"), 0o644))

	require.NoError(t, WriteAppEnv(tempDir, "prod"))

	content, err := os.ReadFile(envLocal)
	require.NoError(t, err)
	assert.Equal(t, "APP_SECRET=secret\nAPP_ENV=prod\nDATABASE_URL=mysql://...\n", string(content))
}

func TestWriteAppEnv_AppendsWhenMissing(t *testing.T) {
	tempDir := t.TempDir()
	envLocal := filepath.Join(tempDir, ".env.local")
	require.NoError(t, os.WriteFile(envLocal, []byte("APP_SECRET=secret\n"), 0o644))

	require.NoError(t, WriteAppEnv(tempDir, "test"))

	content, err := os.ReadFile(envLocal)
	require.NoError(t, err)
	assert.Equal(t, "APP_SECRET=secret\nAPP_ENV=test\n", string(content))
}

func TestWriteAppEnv_AppendsNewlineWhenMissing(t *testing.T) {
	tempDir := t.TempDir()
	envLocal := filepath.Join(tempDir, ".env.local")
	require.NoError(t, os.WriteFile(envLocal, []byte("APP_SECRET=secret"), 0o644))

	require.NoError(t, WriteAppEnv(tempDir, "prod"))

	content, err := os.ReadFile(envLocal)
	require.NoError(t, err)
	assert.Equal(t, "APP_SECRET=secret\nAPP_ENV=prod\n", string(content))
}

func TestWriteAppEnv_PreservesComments(t *testing.T) {
	tempDir := t.TempDir()
	envLocal := filepath.Join(tempDir, ".env.local")
	require.NoError(t, os.WriteFile(envLocal, []byte("# Local environment\nAPP_ENV=dev\n# trailing comment\n"), 0o644))

	require.NoError(t, WriteAppEnv(tempDir, "test"))

	content, err := os.ReadFile(envLocal)
	require.NoError(t, err)
	assert.Equal(t, "# Local environment\nAPP_ENV=test\n# trailing comment\n", string(content))
}

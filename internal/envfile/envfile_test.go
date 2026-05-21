package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadValue_NoFiles(t *testing.T) {
	tempDir := t.TempDir()

	value, err := ReadValue(tempDir, "APP_ENV")
	require.NoError(t, err)
	assert.Equal(t, "", value)
}

func TestReadValue_FromEnv(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".env"), []byte("APP_ENV=dev\n"), 0o644))

	value, err := ReadValue(tempDir, "APP_ENV")
	require.NoError(t, err)
	assert.Equal(t, "dev", value)
}

func TestReadValue_EnvLocalOverridesEnv(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".env"), []byte("APP_ENV=prod\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".env.local"), []byte("APP_ENV=dev\n"), 0o644))

	value, err := ReadValue(tempDir, "APP_ENV")
	require.NoError(t, err)
	assert.Equal(t, "dev", value)
}

func TestReadValues_ReturnsRequestedKeys(t *testing.T) {
	tempDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tempDir, ".env.local"),
		[]byte("APP_ENV=prod\nAPP_SECRET=abc\nOTHER=ignored\n"), 0o644))

	values, err := ReadValues(tempDir, "APP_ENV", "APP_SECRET", "MISSING")
	require.NoError(t, err)
	assert.Equal(t, "prod", values["APP_ENV"])
	assert.Equal(t, "abc", values["APP_SECRET"])
	assert.Equal(t, "", values["MISSING"])
	assert.NotContains(t, values, "OTHER")
}

func TestReadValues_NoFiles(t *testing.T) {
	tempDir := t.TempDir()

	values, err := ReadValues(tempDir, "APP_ENV", "APP_SECRET")
	require.NoError(t, err)
	assert.Equal(t, "", values["APP_ENV"])
	assert.Equal(t, "", values["APP_SECRET"])
}

func TestWriteValue_CreatesFile(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, WriteValue(tempDir, "APP_ENV", "prod"))

	content, err := os.ReadFile(filepath.Join(tempDir, ".env.local"))
	require.NoError(t, err)
	assert.Equal(t, "APP_ENV=prod\n", string(content))
}

func TestWriteValue_ReplacesExistingLine(t *testing.T) {
	tempDir := t.TempDir()
	envLocal := filepath.Join(tempDir, ".env.local")
	require.NoError(t, os.WriteFile(envLocal,
		[]byte("APP_SECRET=secret\nAPP_ENV=dev\nDATABASE_URL=mysql://...\n"), 0o644))

	require.NoError(t, WriteValue(tempDir, "APP_ENV", "prod"))

	content, err := os.ReadFile(envLocal)
	require.NoError(t, err)
	assert.Equal(t, "APP_SECRET=secret\nAPP_ENV=prod\nDATABASE_URL=mysql://...\n", string(content))
}

func TestWriteValue_AppendsWhenMissing(t *testing.T) {
	tempDir := t.TempDir()
	envLocal := filepath.Join(tempDir, ".env.local")
	require.NoError(t, os.WriteFile(envLocal, []byte("APP_SECRET=secret\n"), 0o644))

	require.NoError(t, WriteValue(tempDir, "APP_ENV", "test"))

	content, err := os.ReadFile(envLocal)
	require.NoError(t, err)
	assert.Equal(t, "APP_SECRET=secret\nAPP_ENV=test\n", string(content))
}

func TestWriteValue_AppendsNewlineWhenMissing(t *testing.T) {
	tempDir := t.TempDir()
	envLocal := filepath.Join(tempDir, ".env.local")
	require.NoError(t, os.WriteFile(envLocal, []byte("APP_SECRET=secret"), 0o644))

	require.NoError(t, WriteValue(tempDir, "APP_ENV", "prod"))

	content, err := os.ReadFile(envLocal)
	require.NoError(t, err)
	assert.Equal(t, "APP_SECRET=secret\nAPP_ENV=prod\n", string(content))
}

func TestWriteValue_PreservesComments(t *testing.T) {
	tempDir := t.TempDir()
	envLocal := filepath.Join(tempDir, ".env.local")
	require.NoError(t, os.WriteFile(envLocal,
		[]byte("# Local environment\nAPP_ENV=dev\n# trailing comment\n"), 0o644))

	require.NoError(t, WriteValue(tempDir, "APP_ENV", "test"))

	content, err := os.ReadFile(envLocal)
	require.NoError(t, err)
	assert.Equal(t, "# Local environment\nAPP_ENV=test\n# trailing comment\n", string(content))
}

func TestWriteValues_BatchReplaceAndAppendDeterministic(t *testing.T) {
	tempDir := t.TempDir()
	envLocal := filepath.Join(tempDir, ".env.local")
	require.NoError(t, os.WriteFile(envLocal, []byte("APP_ENV=dev\n"), 0o644))

	require.NoError(t, WriteValues(tempDir, map[string]string{
		"APP_ENV":      "prod",
		"NEW_FLAG":     "1",
		"ANOTHER_FLAG": "yes",
	}))

	content, err := os.ReadFile(envLocal)
	require.NoError(t, err)
	// Replaces APP_ENV in place; appends new keys in alphabetical order.
	assert.Equal(t, "APP_ENV=prod\nANOTHER_FLAG=yes\nNEW_FLAG=1\n", string(content))
}

func TestWriteValues_EmptyMapIsNoOp(t *testing.T) {
	tempDir := t.TempDir()

	require.NoError(t, WriteValues(tempDir, nil))

	_, err := os.Stat(filepath.Join(tempDir, ".env.local"))
	assert.True(t, os.IsNotExist(err))
}

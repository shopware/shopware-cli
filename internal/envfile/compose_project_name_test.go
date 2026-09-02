package envfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractEnvVar(t *testing.T) {
	t.Parallel()

	content := []byte("# comment\n\nFOO = bar \nAPP_URL=http://127.0.0.1:8000\nAPP_URL=second\nBROKEN\n")

	assert.Equal(t, "bar", ExtractEnvVar(content, "FOO"), "whitespace around key and value is trimmed")
	assert.Equal(t, "http://127.0.0.1:8000", ExtractEnvVar(content, "APP_URL"), "the first assignment wins")
	assert.Empty(t, ExtractEnvVar(content, "APP"), "a key that only shares a prefix does not match")
	assert.Empty(t, ExtractEnvVar(content, "MISSING"))
	assert.Empty(t, ExtractEnvVar(nil, "FOO"))
}

func TestExtractComposeProjectName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "sw-shop-abc123", ExtractComposeProjectName([]byte("FOO=bar\nCOMPOSE_PROJECT_NAME=sw-shop-abc123\nAPP=1\n")))
	assert.Empty(t, ExtractComposeProjectName([]byte("APP_ENV=dev\n")))
	assert.Empty(t, ExtractComposeProjectName(nil))
}

func TestReadComposeProjectName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	assert.Empty(t, ReadComposeProjectName(dir), "missing .env reads as unset")

	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_ENV=dev\nCOMPOSE_PROJECT_NAME=sw-shop-abc123\n"), 0o644))
	assert.Equal(t, "sw-shop-abc123", ReadComposeProjectName(dir))
}

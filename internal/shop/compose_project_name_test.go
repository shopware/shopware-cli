package shop

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/envfile"
)

func TestGenerateComposeProjectName(t *testing.T) {
	t.Parallel()

	name, err := GenerateComposeProjectName("/tmp/my-shop")
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^sw-my-shop-[0-9a-f]{6}$`), name)
	assert.Regexp(t, regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`), name)

	// Same basename must still differ (random suffix).
	name2, err := GenerateComposeProjectName("/other/my-shop")
	require.NoError(t, err)
	assert.NotEqual(t, name, name2)

	// Sanitize invalid basename characters.
	weird, err := GenerateComposeProjectName(filepath.Join(t.TempDir(), "My Shop!"))
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^sw-my-shop-[0-9a-f]{6}$`), weird)
	assert.Regexp(t, regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`), weird)

	// Regression: user-facing names with uppercase, spaces and umlauts are
	// accepted and still yield a valid Compose project name.
	fancy, err := GenerateComposeProjectName(filepath.Join(t.TempDir(), "München Shop"))
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^sw-m-nchen-shop-[0-9a-f]{6}$`), fancy)

	// camelCase folder names are lowercased into a valid Compose project name.
	camel, err := GenerateComposeProjectName(filepath.Join(t.TempDir(), "myShopwareProject"))
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^sw-myshopwareproject-[0-9a-f]{6}$`), camel)
	assert.Regexp(t, regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`), camel)
}

func TestEnvFileContent(t *testing.T) {
	t.Parallel()

	t.Run("non-docker is empty", func(t *testing.T) {
		t.Parallel()
		content, err := EnvFileContent(false, "/tmp/shop")
		require.NoError(t, err)
		assert.Empty(t, content)
	})

	t.Run("docker writes unique compose project name", func(t *testing.T) {
		t.Parallel()
		content, err := EnvFileContent(true, "/tmp/demo-shop")
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(content, envfile.ComposeProjectNameEnvKey+"=sw-demo-shop-"))
		assert.True(t, strings.HasSuffix(content, "\n"))
		assert.Regexp(t, regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`), strings.TrimPrefix(strings.TrimSpace(content), envfile.ComposeProjectNameEnvKey+"="))
	})
}

func TestEnsureComposeProjectName(t *testing.T) {
	t.Parallel()

	t.Run("writes when missing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, EnsureComposeProjectName(dir))

		content, err := os.ReadFile(filepath.Join(dir, ".env"))
		require.NoError(t, err)
		assert.Contains(t, string(content), envfile.ComposeProjectNameEnvKey+"=")
		assert.NotEmpty(t, envfile.ExtractComposeProjectName(content))
	})

	t.Run("preserves existing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_PROJECT_NAME=sw-keep-ffffff\n"), 0o644))

		require.NoError(t, EnsureComposeProjectName(dir))

		content, err := os.ReadFile(filepath.Join(dir, ".env"))
		require.NoError(t, err)
		assert.Equal(t, "sw-keep-ffffff", envfile.ExtractComposeProjectName(content))
	})

	t.Run("appends without clobbering other keys", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("FOO=bar"), 0o644))

		require.NoError(t, EnsureComposeProjectName(dir))

		content, err := os.ReadFile(filepath.Join(dir, ".env"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "FOO=bar")
		assert.Contains(t, string(content), envfile.ComposeProjectNameEnvKey+"=")
	})
}

func TestRestoreComposeProjectName(t *testing.T) {
	t.Parallel()

	t.Run("re-adds a lost name", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		// A recipe reset replaced .env and dropped the key.
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_ENV=prod\n"), 0o644))

		require.NoError(t, RestoreComposeProjectName(dir, "sw-shop-abc123"))

		content, err := os.ReadFile(filepath.Join(dir, ".env"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "APP_ENV=prod")
		assert.Equal(t, "sw-shop-abc123", envfile.ExtractComposeProjectName(content))
	})

	t.Run("empty name is a no-op", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		require.NoError(t, RestoreComposeProjectName(dir, ""))

		_, err := os.Stat(filepath.Join(dir, ".env"))
		assert.True(t, os.IsNotExist(err), "no .env is created for projects without a compose project name")
	})

	t.Run("present value stays untouched", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_PROJECT_NAME=sw-keep-ffffff\n"), 0o644))

		require.NoError(t, RestoreComposeProjectName(dir, "sw-other-000000"))

		content, err := os.ReadFile(filepath.Join(dir, ".env"))
		require.NoError(t, err)
		assert.Equal(t, "sw-keep-ffffff", envfile.ExtractComposeProjectName(content))
	})
}

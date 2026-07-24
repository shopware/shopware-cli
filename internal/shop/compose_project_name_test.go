package shop

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateComposeProjectName(t *testing.T) {
	t.Parallel()

	name, err := GenerateComposeProjectName("/tmp/my-shop")
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^sw-my-shop-[0-9a-f]{6}$`), name)
	assert.NoError(t, ValidateProjectName(name))

	// Same basename must still differ (random suffix).
	name2, err := GenerateComposeProjectName("/other/my-shop")
	require.NoError(t, err)
	assert.NotEqual(t, name, name2)

	// Sanitize invalid basename characters.
	weird, err := GenerateComposeProjectName(filepath.Join(t.TempDir(), "My Shop!"))
	require.NoError(t, err)
	assert.Regexp(t, regexp.MustCompile(`^sw-my-shop-[0-9a-f]{6}$`), weird)
	assert.NoError(t, ValidateProjectName(weird))
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
		assert.True(t, strings.HasPrefix(content, ComposeProjectNameEnvKey+"=sw-demo-shop-"))
		assert.True(t, strings.HasSuffix(content, "\n"))
		assert.NoError(t, ValidateProjectName(strings.TrimPrefix(strings.TrimSpace(content), ComposeProjectNameEnvKey+"=")))
	})
}

func TestExtractComposeProjectName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "sw-shop-abc123", ExtractComposeProjectName([]byte("FOO=bar\nCOMPOSE_PROJECT_NAME=sw-shop-abc123\nAPP=1\n")))
	assert.Empty(t, ExtractComposeProjectName([]byte("APP_ENV=dev\n")))
	assert.Empty(t, ExtractComposeProjectName(nil))
}

func TestEnsureComposeProjectName(t *testing.T) {
	t.Parallel()

	t.Run("writes when missing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, EnsureComposeProjectName(dir))

		content, err := os.ReadFile(filepath.Join(dir, ".env"))
		require.NoError(t, err)
		assert.Contains(t, string(content), ComposeProjectNameEnvKey+"=")
		assert.NotEmpty(t, ExtractComposeProjectName(content))
	})

	t.Run("preserves existing", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_PROJECT_NAME=sw-keep-ffffff\n"), 0o644))

		require.NoError(t, EnsureComposeProjectName(dir))

		content, err := os.ReadFile(filepath.Join(dir, ".env"))
		require.NoError(t, err)
		assert.Equal(t, "sw-keep-ffffff", ExtractComposeProjectName(content))
	})

	t.Run("appends without clobbering other keys", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("FOO=bar"), 0o644))

		require.NoError(t, EnsureComposeProjectName(dir))

		content, err := os.ReadFile(filepath.Join(dir, ".env"))
		require.NoError(t, err)
		assert.Contains(t, string(content), "FOO=bar")
		assert.Contains(t, string(content), ComposeProjectNameEnvKey+"=")
	})
}

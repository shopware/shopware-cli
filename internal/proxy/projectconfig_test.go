package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testProjectConfig = `# my project
url: http://127.0.0.1:8000
compatibility_date: "2026-07-15" # keep me
docker:
    php:
        version: "8.5"
environments:
    local:
        type: docker
        url: http://127.0.0.1:8000
        admin_api:
            username: admin
            password: shopware
`

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), ".shopware-project.yml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestProjectConfigURLRoundTrip(t *testing.T) {
	t.Parallel()

	path := writeTestConfig(t, testProjectConfig)

	state, err := ReadProjectConfigURLs(path, "")
	require.NoError(t, err)
	assert.True(t, state.HasFile)
	assert.True(t, state.HasRoot)
	assert.Equal(t, "http://127.0.0.1:8000", state.RootURL)
	assert.True(t, state.HasEnv)
	assert.Equal(t, "http://127.0.0.1:8000", state.EnvURL)

	require.NoError(t, SetProjectConfigURLs(path, "", "https://my-shop.shopware.local"))

	updated, err := os.ReadFile(path)
	require.NoError(t, err)
	// Both url keys point at the proxy; comments and other keys survive.
	assert.Equal(t, 2, countOccurrences(string(updated), "https://my-shop.shopware.local"))
	assert.Contains(t, string(updated), "# my project")
	assert.Contains(t, string(updated), "# keep me")
	assert.Contains(t, string(updated), "password: shopware")

	require.NoError(t, RestoreProjectConfigURLs(path, "", state))

	restored, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(restored), "my-shop.shopware.local")
	assert.Equal(t, 2, countOccurrences(string(restored), "http://127.0.0.1:8000"))
}

func TestProjectConfigURLAbsentKeysAreRemovedOnRestore(t *testing.T) {
	t.Parallel()

	path := writeTestConfig(t, "compatibility_date: \"2026-07-15\"\n")

	state, err := ReadProjectConfigURLs(path, "")
	require.NoError(t, err)
	assert.True(t, state.HasFile)
	assert.False(t, state.HasRoot)
	assert.False(t, state.HasEnv)

	require.NoError(t, SetProjectConfigURLs(path, "", "https://my-shop.shopware.local"))
	updated, _ := os.ReadFile(path)
	assert.Contains(t, string(updated), "url: https://my-shop.shopware.local")
	// No environments section existed, none is invented.
	assert.NotContains(t, string(updated), "environments")

	require.NoError(t, RestoreProjectConfigURLs(path, "", state))
	restored, _ := os.ReadFile(path)
	assert.NotContains(t, string(restored), "url:")
	assert.Contains(t, string(restored), "compatibility_date")
}

func TestReadProjectConfigURLsMissingFile(t *testing.T) {
	t.Parallel()

	state, err := ReadProjectConfigURLs(filepath.Join(t.TempDir(), "nope.yml"), "")
	require.NoError(t, err)
	assert.False(t, state.HasFile)
}

func countOccurrences(s, sub string) int {
	count := 0
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			count++
		}
	}
	return count
}

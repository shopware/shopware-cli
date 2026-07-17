package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteTraefikDynamicConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Leftovers from earlier versions that must be cleared: a static config
	// file would silently override every CLI flag if it ever reached a
	// container path Traefik auto-loads from.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "traefik", "dynamic"), 0o700))
	for _, stale := range []string{
		filepath.Join(dir, "traefik", "traefik.yml"),
		filepath.Join(dir, "traefik", "traefik.yaml"),
		filepath.Join(dir, "traefik", "dynamic", "tls.yml"),
	} {
		require.NoError(t, os.WriteFile(stale, []byte("stale"), 0o600))
	}

	require.NoError(t, writeTraefikDynamicConfig(dir, "custom.internal"))

	assert.NoFileExists(t, filepath.Join(dir, "traefik", "traefik.yml"))
	assert.NoFileExists(t, filepath.Join(dir, "traefik", "traefik.yaml"))
	assert.NoFileExists(t, filepath.Join(dir, "traefik", "dynamic", "tls.yml"))

	content, err := os.ReadFile(filepath.Join(dir, "traefik", "dynamic", "tls.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(content), "proxy.custom.internal")
	assert.Contains(t, string(content), containerConfigDir+"/certs/cert.pem")
}

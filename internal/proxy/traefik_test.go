package proxy

import (
	"errors"
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

func TestManagedAliases(t *testing.T) {
	t.Parallel()

	// Docker adds the short container-ID alias itself; only dotted hostnames
	// are ours.
	got := managedAliases([]string{"a1b2c3d4e5f6", "shop1.shopware.local", "shopware-cli-proxy", "shop2.shopware.local"})
	assert.ElementsMatch(t, []string{"shop1.shopware.local", "shop2.shopware.local"}, got)
}

func TestEqualStringSets(t *testing.T) {
	t.Parallel()

	assert.True(t, equalStringSets(nil, nil))
	assert.True(t, equalStringSets([]string{"a", "b"}, []string{"b", "a"}))
	assert.False(t, equalStringSets([]string{"a"}, []string{"a", "b"}))
	assert.False(t, equalStringSets([]string{"a", "b"}, []string{"a"}))
}

func TestReadablePortConflictOnlyRewritesPortFailures(t *testing.T) {
	other := errors.New("docker [run -p 80:80]: exit status 125\nno such image")
	assert.Same(t, other, readablePortConflict(other))

	assert.NoError(t, readablePortConflict(nil))

	for name, output := range map[string]string{
		"engine":  "Bind for 0.0.0.0:443 failed: port is already allocated",
		"desktop": "ports are not available: exposing port TCP 0.0.0.0:443 -> 127.0.0.1:0: listen tcp 0.0.0.0:443: bind: address already in use",
	} {
		t.Run(name, func(t *testing.T) {
			err := readablePortConflict(errors.New("docker [run -d -p 80:80 -p 443:443]: exit status 125\n" + output))

			assert.NotContains(t, err.Error(), "docker [run")
			assert.ErrorContains(t, err, "host port 443 ")
		})
	}
}

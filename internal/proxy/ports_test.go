package proxy

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/docker"
	"github.com/shopware/shopware-cli/internal/shop"
)

// writeMinimalComposeProject writes the composer.lock compose generation needs.
func writeMinimalComposeProject(t *testing.T, dir string) {
	t.Helper()
	lock := `{"packages": [{"name": "shopware/core", "version": "6.6.0.0"}], "packages-dev": []}`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(lock), 0o644))
}

func webConflict() []docker.PortConflict {
	return []docker.PortConflict{
		{Service: docker.ServiceWeb, Endpoint: docker.PortHTTP, Label: "Shop (Caddy)", HostPort: 8000},
	}
}

func TestApplyRandomPorts_PersistsAndRewritesCompose(t *testing.T) {
	dir := t.TempDir()
	writeMinimalComposeProject(t, dir)
	configPath := filepath.Join(dir, ".shopware-project.yml")
	cfg := &shop.Config{}

	updated, overrides, err := ApplyRandomPorts(t.Context(), dir, configPath, cfg, false, webConflict())
	require.NoError(t, err)

	require.Len(t, overrides, 1)
	port := overrides[0].HostPort
	assert.Greater(t, port, 0)
	assert.Nil(t, cfg.Docker, "the input config must not be mutated")
	assert.Equal(t, docker.Port(port), updated.DockerPorts(docker.ServiceWeb)[docker.PortHTTP])

	localContent, err := os.ReadFile(filepath.Join(dir, ".shopware-project.local.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(localContent), fmt.Sprintf("http: %d", port))

	composeContent, err := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(composeContent), fmt.Sprintf("%d:8000", port))
}

// A proxy project that fell back to fixed-port serving keeps its proxy URL in
// the config, so resolving the mode from the config would undo the fallback
// and leave the shop without published host ports.
func TestApplyRandomPorts_FallbackProxyKeepsFixedPorts(t *testing.T) {
	dir := t.TempDir()
	writeMinimalComposeProject(t, dir)
	configPath := filepath.Join(dir, ".shopware-project.yml")
	cfg := &shop.Config{URL: "http://myshop." + DefaultDomain}

	_, overrides, err := ApplyRandomPorts(t.Context(), dir, configPath, cfg, true, webConflict())
	require.NoError(t, err)

	composeContent, err := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(composeContent), fmt.Sprintf("%d:8000", overrides[0].HostPort))
	assert.NotContains(t, string(composeContent), "traefik.enable")
}

func TestNewEnvironment_ModeFollowsConfigUnlessFallback(t *testing.T) {
	dir := t.TempDir()
	writeMinimalComposeProject(t, dir)
	cfg := &shop.Config{URL: "http://myshop." + DefaultDomain}

	proxied, err := NewEnvironment(dir, cfg, false)
	require.NoError(t, err)
	assert.Equal(t, "myshop."+DefaultDomain, proxied.ProxyHost())
	require.NoError(t, proxied.WriteCompose())
	composeContent, err := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	require.NoError(t, err)
	assert.Contains(t, string(composeContent), "traefik.enable")

	plain, err := NewEnvironment(dir, cfg, true)
	require.NoError(t, err)
	assert.Empty(t, plain.ProxyHost(), "a fallback forces fixed-port mode")

	portBased, err := NewEnvironment(dir, &shop.Config{URL: "http://127.0.0.1:8000"}, false)
	require.NoError(t, err)
	assert.Empty(t, portBased.ProxyHost())

	_, err = NewEnvironment(t.TempDir(), cfg, false)
	assert.ErrorContains(t, err, "composer.lock", "a project without a lock has no environment")
}

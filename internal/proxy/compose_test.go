package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestComposeOverrideIsValidYaml(t *testing.T) {
	content := ComposeOverride("my-shop", "my-shop.127.0.0.1.sslip.io", "web", 8000)

	var parsed map[string]any
	assert.NoError(t, yaml.Unmarshal([]byte(content), &parsed))

	services := parsed["services"].(map[string]any)
	web := services["web"].(map[string]any)

	labels := web["labels"].(map[string]any)
	assert.Equal(t, "Host(`my-shop.127.0.0.1.sslip.io`)", labels["traefik.http.routers.my-shop.rule"])
	assert.Equal(t, "8000", labels["traefik.http.services.my-shop.loadbalancer.server.port"])
	assert.Equal(t, "my-shop.127.0.0.1.sslip.io", labels[HostLabel])

	assert.ElementsMatch(t, []any{"default", NetworkName}, web["networks"])

	networks := parsed["networks"].(map[string]any)
	assert.Equal(t, map[string]any{"external": true}, networks[NetworkName])
}

func TestWriteComposeOverride(t *testing.T) {
	dir := t.TempDir()

	_, err := WriteComposeOverride(dir, "my-shop", "my-shop.127.0.0.1.sslip.io", "web", 8000)
	assert.Error(t, err, "should fail without a compose file")

	assert.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644))

	path, err := WriteComposeOverride(dir, "my-shop", "my-shop.127.0.0.1.sslip.io", "web", 8000)
	assert.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "docker-compose.override.yml"), path)

	// Regenerating our own file is fine.
	_, err = WriteComposeOverride(dir, "my-shop", "other.127.0.0.1.sslip.io", "web", 8000)
	assert.NoError(t, err)
}

func TestWriteComposeOverrideRefusesForeignFile(t *testing.T) {
	dir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services: {}\n"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "compose.override.yaml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644))

	_, err := WriteComposeOverride(dir, "my-shop", "my-shop.127.0.0.1.sslip.io", "web", 8000)
	assert.Error(t, err)
}

func TestRemoveComposeOverride(t *testing.T) {
	dir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644))

	_, err := RemoveComposeOverride(dir)
	assert.Error(t, err, "nothing to remove")

	path, err := WriteComposeOverride(dir, "my-shop", "my-shop.127.0.0.1.sslip.io", "web", 8000)
	assert.NoError(t, err)

	removed, err := RemoveComposeOverride(dir)
	assert.NoError(t, err)
	assert.Equal(t, path, removed)
	assert.NoFileExists(t, path)
}

func TestRemoveComposeOverrideRefusesForeignFile(t *testing.T) {
	dir := t.TempDir()

	assert.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte("services: {}\n"), 0o644))
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose.override.yml"), []byte("services:\n  web:\n    image: nginx\n"), 0o644))

	_, err := RemoveComposeOverride(dir)
	assert.Error(t, err)
	assert.FileExists(t, filepath.Join(dir, "docker-compose.override.yml"))
}

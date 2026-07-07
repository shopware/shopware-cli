package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestTraefikStaticConfigDefaultPorts(t *testing.T) {
	content := TraefikStaticConfig(DefaultSettings())

	var parsed map[string]any
	assert.NoError(t, yaml.Unmarshal([]byte(content), &parsed))

	providers := parsed["providers"].(map[string]any)
	docker := providers["docker"].(map[string]any)
	assert.Equal(t, false, docker["exposedByDefault"])
	assert.Equal(t, NetworkName, docker["network"])

	entryPoints := parsed["entryPoints"].(map[string]any)
	web := entryPoints["web"].(map[string]any)
	redirections := web["http"].(map[string]any)["redirections"].(map[string]any)["entryPoint"].(map[string]any)
	assert.Equal(t, "websecure", redirections["to"])
}

func TestTraefikStaticConfigCustomHTTPSPort(t *testing.T) {
	settings := DefaultSettings()
	settings.HTTPSPort = 8443

	content := TraefikStaticConfig(settings)

	var parsed map[string]any
	assert.NoError(t, yaml.Unmarshal([]byte(content), &parsed))

	entryPoints := parsed["entryPoints"].(map[string]any)
	web := entryPoints["web"].(map[string]any)
	redirections := web["http"].(map[string]any)["redirections"].(map[string]any)["entryPoint"].(map[string]any)
	assert.Equal(t, ":8443", redirections["to"])
}

func TestWriteTraefikConfig(t *testing.T) {
	dir := t.TempDir()

	assert.NoError(t, WriteTraefikConfig(dir, DefaultSettings()))
	assert.FileExists(t, filepath.Join(dir, "traefik", "traefik.yml"))

	dynamic, err := os.ReadFile(filepath.Join(dir, "traefik", "dynamic", "tls.yml"))
	assert.NoError(t, err)

	var parsed map[string]any
	assert.NoError(t, yaml.Unmarshal(dynamic, &parsed))
	assert.Contains(t, parsed, "tls")
}

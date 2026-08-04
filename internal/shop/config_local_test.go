package shop

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateLocalDockerPortsCreatesFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")

	require.NoError(t, UpdateLocalDockerPorts(configPath, map[string]int{
		DockerPortWeb:       52341,
		DockerPortMailerWeb: 52342,
	}))

	localFile := filepath.Join(dir, ".shopware-project.local.yml")
	info, err := os.Stat(localFile)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	content, err := os.ReadFile(localFile)
	require.NoError(t, err)
	assert.Contains(t, string(content), "web: 52341")
	assert.Contains(t, string(content), "mailer_web: 52342")
}

func TestUpdateLocalDockerPortsPreservesExistingContent(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")
	localFile := filepath.Join(dir, ".shopware-project.local.yml")

	existing := `# local secrets
admin_api:
  username: ${SHOP_USER}
docker:
  php:
    blackfire_server_token: secret-token
  ports:
    web: 9000
`
	require.NoError(t, os.WriteFile(localFile, []byte(existing), 0o600))

	require.NoError(t, UpdateLocalDockerPorts(configPath, map[string]int{
		DockerPortWeb:     52341,
		DockerPortAdminer: 52343,
	}))

	content, err := os.ReadFile(localFile)
	require.NoError(t, err)
	text := string(content)

	assert.Contains(t, text, "# local secrets", "comments must survive")
	assert.Contains(t, text, "${SHOP_USER}", "env references must stay unexpanded")
	assert.Contains(t, text, "blackfire_server_token: secret-token")
	assert.Contains(t, text, "web: 52341", "existing port key must be updated")
	assert.NotContains(t, text, "web: 9000")
	assert.Contains(t, text, "adminer: 52343")
}

func TestUpdateLocalDockerPortsUsesYamlExtension(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yaml")

	require.NoError(t, UpdateLocalDockerPorts(configPath, map[string]int{DockerPortWeb: 52341}))

	_, err := os.Stat(filepath.Join(dir, ".shopware-project.local.yaml"))
	assert.NoError(t, err)
}

func TestUpdateLocalDockerPortsRoundTripsThroughReadConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("url: http://localhost:8000\n"), 0o644))

	require.NoError(t, UpdateLocalDockerPorts(configPath, map[string]int{DockerPortWeb: 52341}))

	cfg, err := ReadConfig(context.Background(), configPath, false)
	require.NoError(t, err)
	require.NotNil(t, cfg.Docker)
	assert.Equal(t, ConfigDockerPort(52341), cfg.Docker.Ports[DockerPortWeb])
}

func TestReadConfigMergesLocalFileWithoutBaseConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")

	require.NoError(t, UpdateLocalDockerPorts(configPath, map[string]int{DockerPortWeb: 52341}))

	cfg, err := ReadConfig(context.Background(), configPath, true)
	require.NoError(t, err)
	require.NotNil(t, cfg.Docker)
	assert.Equal(t, ConfigDockerPort(52341), cfg.Docker.Ports[DockerPortWeb])
	assert.True(t, cfg.IsFallback(), "a lone local override file is not a full project config")
}

func TestReadConfigWithLocalFileKeepsUnquotedDate(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")
	// An unquoted date resolves to !!timestamp when the base config is decoded
	// into a generic map during the local-override merge; it must survive the
	// round-trip as a plain YYYY-MM-DD string.
	require.NoError(t, os.WriteFile(configPath, []byte("compatibility_date: 2026-08-01\n"), 0o644))

	require.NoError(t, UpdateLocalDockerPorts(configPath, map[string]int{DockerPortWeb: 52341}))

	cfg, err := ReadConfig(context.Background(), configPath, false)
	require.NoError(t, err)
	assert.Equal(t, "2026-08-01", cfg.CompatibilityDate)
}

func TestUpdateLocalDockerPHPPreservesPorts(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")

	require.NoError(t, UpdateLocalDockerPorts(configPath, map[string]int{DockerPortWeb: 52341}))
	require.NoError(t, UpdateLocalDockerPHP(configPath, &ConfigDockerPHP{
		BlackfireServerID:    "server-id",
		BlackfireServerToken: "server-token",
	}))

	content, err := os.ReadFile(filepath.Join(dir, ".shopware-project.local.yml"))
	require.NoError(t, err)
	text := string(content)

	assert.Contains(t, text, "web: 52341")
	assert.Contains(t, text, "blackfire_server_id: server-id")
	assert.Contains(t, text, "blackfire_server_token: server-token")
	assert.NotContains(t, text, "tideways_api_key", "empty secrets must not be written")
}

func TestReadConfigDockerPortValues(t *testing.T) {
	readConfigWithPorts := func(t *testing.T, portsYaml string) (*Config, error) {
		t.Helper()
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".shopware-project.yml")
		content := "docker:\n  ports:\n" + portsYaml
		require.NoError(t, os.WriteFile(configPath, []byte(content), 0o644))
		return ReadConfig(context.Background(), configPath, false)
	}

	t.Run("false disables the port", func(t *testing.T) {
		cfg, err := readConfigWithPorts(t, "    adminer: false\n    web: 8005\n")
		require.NoError(t, err)
		assert.Equal(t, DockerPortDisabled, cfg.Docker.Ports[DockerPortAdminer])
		assert.True(t, cfg.Docker.Ports[DockerPortAdminer].Disabled())
		assert.Equal(t, ConfigDockerPort(8005), cfg.Docker.Ports[DockerPortWeb])
	})

	t.Run("true is rejected", func(t *testing.T) {
		_, err := readConfigWithPorts(t, "    adminer: true\n")
		assert.ErrorContains(t, err, "use a port number or false")
	})

	t.Run("non-numeric value is rejected", func(t *testing.T) {
		_, err := readConfigWithPorts(t, "    adminer: banana\n")
		assert.ErrorContains(t, err, "expected a port number or false")
	})

	t.Run("out-of-range port is rejected", func(t *testing.T) {
		_, err := readConfigWithPorts(t, "    adminer: 70000\n")
		assert.ErrorContains(t, err, "not a valid port number")
	})

	t.Run("disabled port survives the local-override merge", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".shopware-project.yml")
		require.NoError(t, os.WriteFile(configPath, []byte("docker:\n  ports:\n    adminer: false\n"), 0o644))
		require.NoError(t, UpdateLocalDockerPorts(configPath, map[string]int{DockerPortWeb: 52341}))

		cfg, err := ReadConfig(context.Background(), configPath, false)
		require.NoError(t, err)
		assert.True(t, cfg.Docker.Ports[DockerPortAdminer].Disabled())
		assert.Equal(t, ConfigDockerPort(52341), cfg.Docker.Ports[DockerPortWeb])
	})
}

func TestSetDockerPortOverrides(t *testing.T) {
	cfg := &Config{}
	cfg.SetDockerPortOverrides(map[string]int{DockerPortWeb: 52341})
	require.NotNil(t, cfg.Docker)
	assert.Equal(t, ConfigDockerPort(52341), cfg.Docker.Ports[DockerPortWeb])

	cfg.SetDockerPortOverrides(map[string]int{DockerPortMailerWeb: 52342})
	assert.Equal(t, ConfigDockerPort(52341), cfg.Docker.Ports[DockerPortWeb], "existing overrides must be kept")
	assert.Equal(t, ConfigDockerPort(52342), cfg.Docker.Ports[DockerPortMailerWeb])
}

func TestWriteConfigOmitsPortOverrides(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		URL: "http://localhost:8000",
		Docker: &ConfigDocker{
			PHP:   &ConfigDockerPHP{Version: "8.3"},
			Ports: ConfigDockerPorts{DockerPortWeb: 52341},
		},
	}

	require.NoError(t, WriteConfig(cfg, dir))

	content, err := os.ReadFile(filepath.Join(dir, ".shopware-project.yml"))
	require.NoError(t, err)
	text := string(content)

	assert.Contains(t, text, "version: \"8.3\"")
	assert.NotContains(t, text, "ports", "machine-local port overrides must not land in the committed config")
	assert.NotNil(t, cfg.Docker.Ports, "the caller's config must not be mutated")
}

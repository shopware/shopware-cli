package shop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/docker"
)

func TestUpdateLocalDockerPortsCreatesFile(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")

	require.NoError(t, UpdateLocalDockerPorts(configPath, []docker.PortOverride{
		{Service: docker.ServiceWeb, Endpoint: docker.PortHTTP, HostPort: 52341},
		{Service: docker.ServiceMailer, Endpoint: docker.PortHTTP, HostPort: 52342},
	}))

	localFile := filepath.Join(dir, ".shopware-project.local.yml")
	info, err := os.Stat(localFile)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	content, err := os.ReadFile(localFile)
	require.NoError(t, err)
	assert.Equal(t, "docker:\n    services:\n        mailer:\n            ports:\n                http: 52342\n        web:\n            ports:\n                http: 52341\n", string(content))
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
  services:
    web:
      ports:
        http: 9000
`
	require.NoError(t, os.WriteFile(localFile, []byte(existing), 0o600))

	require.NoError(t, UpdateLocalDockerPorts(configPath, []docker.PortOverride{
		{Service: docker.ServiceWeb, Endpoint: docker.PortHTTP, HostPort: 52341},
		{Service: docker.ServiceAdminer, Endpoint: docker.PortHTTP, HostPort: 52343},
	}))

	content, err := os.ReadFile(localFile)
	require.NoError(t, err)
	text := string(content)

	assert.Contains(t, text, "# local secrets", "comments must survive")
	assert.Contains(t, text, "${SHOP_USER}", "env references must stay unexpanded")
	assert.Contains(t, text, "blackfire_server_token: secret-token")
	assert.Contains(t, text, "http: 52341", "existing port key must be updated")
	assert.NotContains(t, text, "http: 9000")
	assert.Contains(t, text, "adminer:")
	assert.Contains(t, text, "http: 52343")
}

func TestUpdateLocalDockerPortsUsesYamlExtension(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yaml")

	require.NoError(t, UpdateLocalDockerPorts(configPath, webOverride(52341)))

	_, err := os.Stat(filepath.Join(dir, ".shopware-project.local.yaml"))
	assert.NoError(t, err)
}

func TestUpdateLocalDockerPortsRoundTripsThroughReadConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("url: http://localhost:8000\n"), 0o644))

	require.NoError(t, UpdateLocalDockerPorts(configPath, webOverride(52341)))

	cfg, err := ReadConfig(t.Context(), configPath, false)
	require.NoError(t, err)
	assert.Equal(t, docker.Port(52341), cfg.DockerPorts(docker.ServiceWeb)[docker.PortHTTP])
}

func TestReadConfigMergesLocalFileWithoutBaseConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")

	require.NoError(t, UpdateLocalDockerPorts(configPath, webOverride(52341)))

	cfg, err := ReadConfig(t.Context(), configPath, true)
	require.NoError(t, err)
	assert.Equal(t, docker.Port(52341), cfg.DockerPorts(docker.ServiceWeb)[docker.PortHTTP])
	assert.True(t, cfg.IsFallback(), "a lone local override file is not a full project config")
}

func TestReadConfigWithLocalFileKeepsUnquotedDate(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")
	// An unquoted date resolves to !!timestamp when the base config is decoded
	// into a generic map during the local-override merge; it must survive the
	// round-trip as a plain YYYY-MM-DD string.
	require.NoError(t, os.WriteFile(configPath, []byte("compatibility_date: 2026-08-01\n"), 0o644))

	require.NoError(t, UpdateLocalDockerPorts(configPath, webOverride(52341)))

	cfg, err := ReadConfig(t.Context(), configPath, false)
	require.NoError(t, err)
	assert.Equal(t, "2026-08-01", cfg.CompatibilityDate)
}

func TestUpdateLocalDockerPHPPreservesPorts(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")

	require.NoError(t, UpdateLocalDockerPorts(configPath, webOverride(52341)))
	require.NoError(t, UpdateLocalDockerPHP(configPath, &ConfigDockerPHP{
		BlackfireServerID:    "server-id",
		BlackfireServerToken: "server-token",
	}))

	content, err := os.ReadFile(filepath.Join(dir, ".shopware-project.local.yml"))
	require.NoError(t, err)
	text := string(content)

	assert.Contains(t, text, "http: 52341")
	assert.Contains(t, text, "blackfire_server_id: server-id")
	assert.Contains(t, text, "blackfire_server_token: server-token")
	assert.NotContains(t, text, "tideways_api_key", "empty secrets must not be written")
}

func TestUpdateLocalDockerPHPRemovesStaleSecrets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")
	localFile := filepath.Join(dir, ".shopware-project.local.yml")

	require.NoError(t, UpdateLocalDockerPHP(configPath, &ConfigDockerPHP{
		BlackfireServerID:    "server-id",
		BlackfireServerToken: "server-token",
		TidewaysAPIKey:       "tideways-key",
	}))

	// Switching to Tideways must drop the Blackfire credentials.
	require.NoError(t, UpdateLocalDockerPHP(configPath, &ConfigDockerPHP{
		TidewaysAPIKey: "tideways-key",
	}))

	content, err := os.ReadFile(localFile)
	require.NoError(t, err)
	text := string(content)
	assert.Contains(t, text, "tideways_api_key: tideways-key")
	assert.NotContains(t, text, "blackfire_server_id", "rotated credentials must not survive")
	assert.NotContains(t, text, "blackfire_server_token", "rotated credentials must not survive")

	// Disabling the profiler (nil config) must clear every known secret.
	require.NoError(t, UpdateLocalDockerPHP(configPath, nil))

	content, err = os.ReadFile(localFile)
	require.NoError(t, err)
	text = string(content)
	assert.NotContains(t, text, "tideways_api_key", "disabled credentials must not survive")
	assert.NotContains(t, text, "blackfire_server_id")
	assert.NotContains(t, text, "blackfire_server_token")
}

func TestUpdateLocalConfigEnforcesModeOnExistingFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, ".shopware-project.yml")
	localFile := filepath.Join(dir, ".shopware-project.local.yml")

	// Simulate a pre-existing local override created with loose permissions.
	require.NoError(t, os.WriteFile(localFile, []byte("docker: {}\n"), 0o644))

	require.NoError(t, UpdateLocalDockerPHP(configPath, &ConfigDockerPHP{
		TidewaysAPIKey: "tideways-key",
	}))

	info, err := os.Stat(localFile)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "secrets must not stay world-readable on an existing file")
}

func TestReadConfigDockerServices(t *testing.T) {
	readConfig := func(t *testing.T, servicesYaml string) (*Config, error) {
		t.Helper()
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".shopware-project.yml")
		content := "docker:\n  services:\n" + servicesYaml
		require.NoError(t, os.WriteFile(configPath, []byte(content), 0o644))
		return ReadConfig(t.Context(), configPath, false)
	}

	t.Run("false disables the port", func(t *testing.T) {
		cfg, err := readConfig(t, "    adminer:\n      ports:\n        http: false\n    web:\n      ports:\n        http: 8005\n")
		require.NoError(t, err)
		assert.True(t, cfg.DockerPorts(docker.ServiceAdminer)[docker.PortHTTP].Disabled())
		assert.Equal(t, docker.Port(8005), cfg.DockerPorts(docker.ServiceWeb)[docker.PortHTTP])
	})

	t.Run("true is rejected", func(t *testing.T) {
		_, err := readConfig(t, "    adminer:\n      ports:\n        http: true\n")
		assert.ErrorContains(t, err, "use a port number or false")
	})

	t.Run("non-numeric value is rejected", func(t *testing.T) {
		_, err := readConfig(t, "    adminer:\n      ports:\n        http: banana\n")
		assert.ErrorContains(t, err, "expected a port number or false")
	})

	t.Run("out-of-range port is rejected", func(t *testing.T) {
		_, err := readConfig(t, "    adminer:\n      ports:\n        http: 70000\n")
		assert.ErrorContains(t, err, "not a valid port number")
	})

	t.Run("unknown service is rejected", func(t *testing.T) {
		_, err := readConfig(t, "    postgres:\n      ports:\n        http: 5432\n")
		assert.ErrorContains(t, err, "docker.services.postgres: unknown service, valid services: web, database, adminer, mailer, queue, search, storage")
	})

	t.Run("service without settings is rejected", func(t *testing.T) {
		_, err := readConfig(t, "    cache:\n      ports:\n        redis: 6379\n")
		assert.ErrorContains(t, err, "docker.services.cache: unknown service")
	})

	t.Run("unknown port is rejected", func(t *testing.T) {
		_, err := readConfig(t, "    mailer:\n      ports:\n        web: 8025\n")
		assert.ErrorContains(t, err, "docker.services.mailer.ports.web: unknown port, valid ports: smtp, http")
	})

	t.Run("type on a service without variants is rejected", func(t *testing.T) {
		_, err := readConfig(t, "    mailer:\n      type: postfix\n")
		assert.ErrorContains(t, err, "docker.services.mailer: type and version are not configurable")
	})

	t.Run("unknown type is rejected", func(t *testing.T) {
		_, err := readConfig(t, "    database:\n      type: postgres\n")
		assert.ErrorContains(t, err, "docker.services.database.type: unknown value \"postgres\", valid values: mariadb, mysql")
	})

	t.Run("database variant is accepted", func(t *testing.T) {
		cfg, err := readConfig(t, "    database:\n      type: mysql\n      version: \"8.4\"\n      ports:\n        mysql: 3306\n")
		require.NoError(t, err)
		assert.Equal(t, "mysql", cfg.DockerServices()[docker.ServiceDatabase].Type)
		assert.Equal(t, "8.4", cfg.DockerServices()[docker.ServiceDatabase].Version)
		assert.Equal(t, docker.Port(3306), cfg.DockerPorts(docker.ServiceDatabase)[docker.PortMySQL])
	})

	t.Run("empty service entry is accepted", func(t *testing.T) {
		cfg, err := readConfig(t, "    adminer:\n")
		require.NoError(t, err)
		assert.Nil(t, cfg.DockerPorts(docker.ServiceAdminer))
	})

	t.Run("disabled port survives the local-override merge", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".shopware-project.yml")
		require.NoError(t, os.WriteFile(configPath, []byte("docker:\n  services:\n    adminer:\n      ports:\n        http: false\n"), 0o644))
		require.NoError(t, UpdateLocalDockerPorts(configPath, webOverride(52341)))

		cfg, err := ReadConfig(t.Context(), configPath, false)
		require.NoError(t, err)
		assert.True(t, cfg.DockerPorts(docker.ServiceAdminer)[docker.PortHTTP].Disabled())
		assert.Equal(t, docker.Port(52341), cfg.DockerPorts(docker.ServiceWeb)[docker.PortHTTP])
	})
}

func TestWithDockerPortOverrides(t *testing.T) {
	base := &Config{}
	assert.Same(t, base, base.WithDockerPortOverrides(nil), "no overrides returns the receiver")

	first := base.WithDockerPortOverrides(webOverride(52341))
	require.NotNil(t, first.Docker)
	assert.Equal(t, docker.Port(52341), first.DockerPorts(docker.ServiceWeb)[docker.PortHTTP])
	assert.Nil(t, base.Docker, "the receiver must not be mutated")

	second := first.WithDockerPortOverrides([]docker.PortOverride{{Service: docker.ServiceMailer, Endpoint: docker.PortHTTP, HostPort: 52342}})
	assert.Equal(t, docker.Port(52341), second.DockerPorts(docker.ServiceWeb)[docker.PortHTTP], "existing overrides must be kept")
	assert.Equal(t, docker.Port(52342), second.DockerPorts(docker.ServiceMailer)[docker.PortHTTP])
	assert.Nil(t, first.DockerPorts(docker.ServiceMailer), "the receiver's services map must not be shared")

	third := second.WithDockerPortOverrides([]docker.PortOverride{{Service: docker.ServiceWeb, Endpoint: docker.PortHTTPAlt, HostPort: 52343}})
	assert.Equal(t, docker.Port(52343), third.DockerPorts(docker.ServiceWeb)[docker.PortHTTPAlt])
	assert.NotContains(t, second.DockerPorts(docker.ServiceWeb), docker.PortHTTPAlt, "the receiver's ports map must not be shared")

	withSettings := &Config{Docker: &ConfigDocker{
		PHP:      &ConfigDockerPHP{Version: "8.3"},
		Services: ConfigDockerServices{docker.ServiceDatabase: {Type: "mysql"}},
	}}
	merged := withSettings.WithDockerPortOverrides(webOverride(1))
	assert.Equal(t, "8.3", merged.Docker.PHP.Version, "other docker settings are carried over")
	assert.Equal(t, "mysql", merged.DockerServices()[docker.ServiceDatabase].Type)
}

func TestWriteConfigOmitsPortOverrides(t *testing.T) {
	dir := t.TempDir()

	cfg := &Config{
		URL: "http://localhost:8000",
		Docker: &ConfigDocker{
			PHP: &ConfigDockerPHP{Version: "8.3"},
			Services: ConfigDockerServices{
				docker.ServiceWeb:      {Ports: docker.Ports{docker.PortHTTP: 52341}},
				docker.ServiceDatabase: {Type: "mysql", Ports: docker.Ports{docker.PortMySQL: 3306}},
				docker.ServiceAdminer:  nil, // "adminer:" without a value
			},
		},
	}

	require.NoError(t, WriteConfig(cfg, dir))

	content, err := os.ReadFile(filepath.Join(dir, ".shopware-project.yml"))
	require.NoError(t, err)
	text := string(content)

	assert.Contains(t, text, "version: \"8.3\"")
	assert.Contains(t, text, "type: mysql", "project-level service settings are committed")
	assert.NotContains(t, text, "ports", "machine-local port overrides must not land in the committed config")
	assert.NotContains(t, text, "web:", "a service left without settings is dropped")
	assert.Contains(t, text, "adminer:", "an entry without settings is kept as written")
	assert.NotNil(t, cfg.DockerPorts(docker.ServiceWeb), "the caller's config must not be mutated")
	assert.NotNil(t, cfg.DockerPorts(docker.ServiceDatabase))
}

func webOverride(port int) []docker.PortOverride {
	return []docker.PortOverride{{Service: docker.ServiceWeb, Endpoint: docker.PortHTTP, HostPort: port}}
}

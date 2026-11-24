package shop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const environmentsFixture = `# project config
compatibility_date: "2026-01-01"
environments:
  local:
    type: local
    url: http://localhost:8000 # dev shop
  staging:
    type: ssh
    url: https://staging.example.com
    ssh:
      host: ${STAGING_HOST}
      # deploy user
      user: deploy
      port: 2222
      directory: /var/www/shop/current
      hooks:
        post:
          - echo done
`

func writeEnvironmentsFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shopware-project.yml")
	require.NoError(t, os.WriteFile(path, []byte(environmentsFixture), 0o644))
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

func sshEnvironment(host, directory string) *EnvironmentConfig {
	return &EnvironmentConfig{Type: "ssh", URL: "https://" + host, SSH: &EnvironmentSSHConfig{Host: host, Directory: directory}}
}

func TestAddEnvironmentKeepsTheRestOfTheFile(t *testing.T) {
	path := writeEnvironmentsFixture(t)

	require.NoError(t, AddEnvironment(path, "preprod", sshEnvironment("preprod.example.com", "/srv/shop/current")))

	assert.Equal(t, environmentsFixture+`  preprod:
    type: ssh
    url: https://preprod.example.com
    ssh:
      host: preprod.example.com
      directory: /srv/shop/current
`, readFile(t, path))

	t.Setenv("STAGING_HOST", "staging.example.com")
	cfg, err := ReadConfig(t.Context(), path, false)
	require.NoError(t, err)
	assert.Equal(t, "preprod.example.com", cfg.Environments["preprod"].SSH.Host)
}

func TestAddEnvironmentCreatesEnvironmentsKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shopware-project.yml")
	require.NoError(t, os.WriteFile(path, []byte("compatibility_date: \"2026-01-01\"\n"), 0o644))

	require.NoError(t, AddEnvironment(path, "production", sshEnvironment("shop.example.com", "/srv/shop/current")))

	cfg, err := ReadConfig(t.Context(), path, false)
	require.NoError(t, err)
	assert.Equal(t, "/srv/shop/current", cfg.Environments["production"].SSH.Directory)
}

func TestAddEnvironmentRejectsInvalidInput(t *testing.T) {
	for name, tc := range map[string]struct {
		name string
		env  *EnvironmentConfig
	}{
		"existing name":     {"staging", sshEnvironment("other.example.com", "/srv/shop")},
		"name with space":   {"pre prod", sshEnvironment("other.example.com", "/srv/shop")},
		"leading dash":      {"-prod", sshEnvironment("other.example.com", "/srv/shop")},
		"missing directory": {"preprod", sshEnvironment("other.example.com", "")},
		"missing ssh":       {"preprod", &EnvironmentConfig{Type: "ssh"}},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeEnvironmentsFixture(t)
			require.Error(t, AddEnvironment(path, tc.name, tc.env))
			assert.Equal(t, environmentsFixture, readFile(t, path))
		})
	}
}

func TestAddEnvironmentRequiresConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yml")
	require.Error(t, AddEnvironment(path, "preprod", sshEnvironment("preprod.example.com", "/srv/shop")))
	assert.NoFileExists(t, path)
}

func TestUpdateEnvironmentChangesOnlyGivenSettings(t *testing.T) {
	t.Setenv("STAGING_HOST", "staging.example.com")
	path := writeEnvironmentsFixture(t)

	require.NoError(t, UpdateEnvironment(path, "staging", EnvironmentChange{
		Directory:    new("/srv/shop/current"),
		Port:         new(0),
		User:         new(""),
		IdentityFile: new("~/.ssh/deploy"),
	}))

	assert.Equal(t, `# project config
compatibility_date: "2026-01-01"
environments:
  local:
    type: local
    url: http://localhost:8000 # dev shop
  staging:
    type: ssh
    url: https://staging.example.com
    ssh:
      host: ${STAGING_HOST}
      directory: /srv/shop/current
      hooks:
        post:
          - echo done
      identity_file: ~/.ssh/deploy
`, readFile(t, path))
}

func TestUpdateEnvironmentWritesPortAsNumber(t *testing.T) {
	path := writeEnvironmentsFixture(t)
	t.Setenv("STAGING_HOST", "staging.example.com")

	require.NoError(t, UpdateEnvironment(path, "staging", EnvironmentChange{Port: new(2200)}))

	cfg, err := ReadConfig(t.Context(), path, false)
	require.NoError(t, err)
	assert.Equal(t, 2200, cfg.Environments["staging"].SSH.Port)
}

func TestUpdateEnvironmentResolvesReferencesWithoutWritingThem(t *testing.T) {
	t.Setenv("STAGING_HOST", "staging.example.com")
	t.Setenv("STAGING_PORT", "2222")
	path := writeEnvironmentsFixture(t)
	content := strings.Replace(environmentsFixture, "port: 2222", "port: ${STAGING_PORT}", 1)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	require.NoError(t, UpdateEnvironment(path, "staging", EnvironmentChange{User: new("new-deploy")}))
	assert.Contains(t, readFile(t, path), "port: ${STAGING_PORT}")
	cfg, err := ReadConfig(t.Context(), path, false)
	require.NoError(t, err)
	assert.Equal(t, 2222, cfg.Environments["staging"].SSH.Port)
	assert.Equal(t, "new-deploy", cfg.Environments["staging"].SSH.User)
}

func TestUpdateEnvironmentRenames(t *testing.T) {
	path := writeEnvironmentsFixture(t)
	t.Setenv("STAGING_HOST", "staging.example.com")

	require.NoError(t, UpdateEnvironment(path, "staging", EnvironmentChange{Name: new("preprod"), URL: new("https://preprod.example.com")}))

	cfg, err := ReadConfig(t.Context(), path, false)
	require.NoError(t, err)
	assert.NotContains(t, cfg.Environments, "staging")
	assert.Equal(t, "https://preprod.example.com", cfg.Environments["preprod"].URL)
	assert.Equal(t, "deploy", cfg.Environments["preprod"].SSH.User)
}

func TestUpdateEnvironmentRejectsInvalidChange(t *testing.T) {
	t.Setenv("STAGING_HOST", "staging.example.com")
	for name, tc := range map[string]struct {
		environment string
		change      EnvironmentChange
	}{
		"unknown environment": {"production", EnvironmentChange{URL: new("https://shop.example.com")}},
		"rename to existing":  {"staging", EnvironmentChange{Name: new("local")}},
		"rename to invalid":   {"staging", EnvironmentChange{Name: new("pre prod")}},
		"removed directory":   {"staging", EnvironmentChange{Directory: new("")}},
		"removed host":        {"staging", EnvironmentChange{Host: new("")}},
	} {
		t.Run(name, func(t *testing.T) {
			path := writeEnvironmentsFixture(t)
			require.Error(t, UpdateEnvironment(path, tc.environment, tc.change))
			assert.Equal(t, environmentsFixture, readFile(t, path))
		})
	}
}

func TestRemoveEnvironment(t *testing.T) {
	path := writeEnvironmentsFixture(t)

	require.NoError(t, RemoveEnvironment(path, "staging"))

	assert.Equal(t, `# project config
compatibility_date: "2026-01-01"
environments:
  local:
    type: local
    url: http://localhost:8000 # dev shop
`, readFile(t, path))

	require.Error(t, RemoveEnvironment(path, "staging"))
}

func TestRemoveEnvironmentRejectsRemovingAnAnchorUsedByAnotherEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shopware-project.yml")
	const content = `compatibility_date: "2026-01-01"
environments:
  staging:
    type: ssh
    ssh:
      host: staging.example.com
      directory: &project_dir /srv/shop/current
  production:
    type: ssh
    ssh:
      host: shop.example.com
      directory: *project_dir
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	require.Error(t, RemoveEnvironment(path, "staging"))
	assert.Equal(t, content, readFile(t, path))
	require.NoError(t, RemoveEnvironment(path, "production"))
	require.NoError(t, RemoveEnvironment(path, "staging"))
}

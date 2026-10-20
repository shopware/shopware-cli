package shop

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestReadConfigValidatesSSHEnvironments(t *testing.T) {
	for name, settings := range map[string]string{
		"missing section":        "{type: ssh}",
		"missing host":           "{type: ssh, ssh: {directory: /srv/shop}}",
		"missing directory":      "{type: ssh, ssh: {host: one}}",
		"empty host name":        "{type: ssh, ssh: {directory: /srv/shop, hosts: {'': {host: one}}}}",
		"null host":              "{type: ssh, ssh: {directory: /srv/shop, hosts: {web: null}}}",
		"missing migration host": "{type: ssh, ssh: {directory: /srv/shop, hosts: {a: {host: one}, b: {host: two}}}}",
		"unknown migration host": "{type: ssh, ssh: {migration_host: other, directory: /srv/shop, hosts: {a: {host: one}}}}",
		"duplicate target":       "{type: ssh, ssh: {migration_host: a, directory: /srv/shop, hosts: {a: {host: one}, b: {host: one, port: 22}}}}",
		"invalid parallelism":    "{type: ssh, ssh: {host: one, directory: /srv/shop, parallelism: -1}}",
		"invalid shared path":    "{type: ssh, ssh: {host: one, directory: /srv/shop, shared: {files: [../outside]}}}",
		"SSH block without type": "{ssh: {host: one}}",
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "project.yml")
			testhelper.WriteFile(t, file, "compatibility_date: '2026-01-01'\nenvironments:\n  production: "+settings+"\n")
			cfg, err := ReadConfig(t.Context(), file, false)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.ErrorContains(t, err, "production")
			assert.ErrorContains(t, err, file)
		})
	}
}

func TestReadConfigValidatesSSHAfterAllIncludesAndOverrides(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	testhelper.WriteFile(t, "project.yml", `
compatibility_date: '2026-01-01'
include: [hosts.yml]
environments:
  production:
    type: ssh
    ssh:
      directory: /srv/shop
      migration_host: missing
`)
	testhelper.WriteFile(t, "project.local.yml", `
environments:
  production:
    ssh:
      migration_host: b
`)
	testhelper.WriteFile(t, "hosts.yml", `
include: [transport.yml]
environments:
  production:
    ssh:
      hosts:
        a: {host: one}
        b: {host: one}
`)
	testhelper.WriteFile(t, "hosts.local.yml", `
environments:
  production:
    ssh:
      hosts:
        b: {host: two}
`)
	testhelper.WriteFile(t, "transport.yml", `
environments:
  production:
    ssh:
      user: deploy
      port: 2222
`)
	cfg, err := ReadConfig(t.Context(), "project.yml", false)
	require.NoError(t, err)
	hosts, migration, err := cfg.Environments["production"].SSH.ResolveHosts()
	require.NoError(t, err)
	assert.Equal(t, "b", migration)
	require.Len(t, hosts, 2)
	assert.Equal(t, "one", hosts[0].Config.Host)
	assert.Equal(t, "two", hosts[1].Config.Host)
	for _, host := range hosts {
		assert.Equal(t, "/srv/shop", host.Config.Directory)
		assert.Equal(t, "deploy", host.Config.User)
		assert.Equal(t, 2222, host.Config.Port)
	}

	// A final override that leaves duplicate targets must still be rejected.
	testhelper.WriteFile(t, "hosts.local.yml", "environments:\n  production:\n    ssh:\n      hosts:\n        b: {host: one}\n")
	_, err = ReadConfig(t.Context(), "project.yml", false)
	require.Error(t, err)
}

func TestReadConfigValidatesSSHInFallbackOverride(t *testing.T) {
	file := filepath.Join(t.TempDir(), "project.yml")
	local := LocalConfigFileName(file)
	testhelper.WriteFile(t, local, "environments:\n  production: {type: ssh, ssh: {host: one}}\n")
	_, err := ReadConfig(t.Context(), file, true)
	require.Error(t, err)
	testhelper.WriteFile(t, local, "environments:\n  production: {type: ssh, ssh: {host: one, directory: /srv/shop}}\n")
	cfg, err := ReadConfig(t.Context(), file, true)
	require.NoError(t, err)
	assert.Equal(t, "/srv/shop", cfg.Environments["production"].SSH.Directory)
}

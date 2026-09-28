package project

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func sshGroupTestEnvironment() *shop.EnvironmentConfig {
	return &shop.EnvironmentConfig{Type: executor.TypeSSH, SSH: &shop.EnvironmentSSHConfig{
		User: "deploy", Directory: "/srv/shop/current", MigrationHost: "web-1",
		Hosts: map[string]*shop.EnvironmentSSHHostConfig{
			"web-1": {Host: "web1.example.com"},
			"web-2": {Host: "web2.example.com", PHPBinary: "/usr/bin/php8.3"},
		},
	}}
}

func TestSelectSSHHostNeverBroadcastsOrdinaryCommands(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("ssh-host", "", "")
	env := sshGroupTestEnvironment()
	_, err := selectProjectSSHHost(cmd, env, true)
	require.ErrorContains(t, err, "--ssh-host")
	group, err := selectProjectSSHHost(cmd, env, false)
	require.NoError(t, err)
	assert.Same(t, env, group)
	require.NoError(t, cmd.Flags().Set("ssh-host", "web-2"))
	selected, err := selectProjectSSHHost(cmd, env, true)
	require.NoError(t, err)
	assert.Equal(t, "web2.example.com", selected.SSH.Host)
	assert.Equal(t, "deploy", selected.SSH.User)
	assert.Equal(t, "/usr/bin/php8.3", selected.SSH.PHPBinary)
	assert.Empty(t, selected.SSH.Hosts)
	assert.Len(t, env.SSH.Hosts, 2)
	require.NoError(t, cmd.Flags().Set("ssh-host", "missing"))
	_, err = selectProjectSSHHost(cmd, env, true)
	require.ErrorContains(t, err, `unknown SSH host "missing"`)
}

func TestSelectSSHHostPreservesSingleHostBehavior(t *testing.T) {
	cmd := &cobra.Command{}
	for _, env := range []*shop.EnvironmentConfig{
		{Type: executor.TypeLocal},
		{Type: executor.TypeSSH, SSH: &shop.EnvironmentSSHConfig{Host: "example.com", Directory: "/srv/shop/current"}},
	} {
		selected, err := selectProjectSSHHost(cmd, env, true)
		require.NoError(t, err)
		assert.Same(t, env, selected)
	}
	cmd.Flags().String("ssh-host", "web-1", "")
	_, err := selectProjectSSHHost(cmd, &shop.EnvironmentConfig{Type: executor.TypeLocal}, true)
	require.ErrorContains(t, err, "requires an SSH environment")
}

func TestSSHHostFlagDoesNotUseDatabaseHostFlag(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("host", "database.example.com", "")
	env := &shop.EnvironmentConfig{Type: executor.TypeSSH, SSH: &shop.EnvironmentSSHConfig{Host: "ssh.example.com", Directory: "/srv/shop/current"}}
	selected, err := selectProjectSSHHost(cmd, env, true)
	require.NoError(t, err)
	assert.Equal(t, "ssh.example.com", selected.SSH.Host)
}

func TestSSHHostCompletion(t *testing.T) {
	oldConfig, oldEnvironment := projectConfigPath, environmentName
	t.Cleanup(func() { projectConfigPath, environmentName = oldConfig, oldEnvironment })
	projectConfigPath = filepath.Join(t.TempDir(), "project.yml")
	environmentName = "production"
	testhelper.WriteFile(t, projectConfigPath, `
compatibility_date: "2026-01-01"
environments:
  production:
    type: ssh
    ssh:
      user: deploy
      directory: /srv/shop/current
      migration_host: web-1
      hosts:
        web-2: {host: web2.example.com}
        web-1: {host: web1.example.com}
`)
	cmd := &cobra.Command{}
	cmd.SetContext(t.Context())
	names, directive := sshHostCompletions(cmd, nil, "web-")
	assert.Equal(t, []string{"web-1", "web-2"}, names)
	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	names, _ = sshHostCompletions(cmd, nil, "web-2")
	assert.Equal(t, []string{"web-2"}, names)
}

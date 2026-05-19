package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestSSHDeployerValidate(t *testing.T) {
	envCfg := &shop.EnvironmentConfig{Type: "ssh", SSH: &shop.EnvironmentSSHConfig{}}
	d := &SSHDeployer{exec: &SSHExecutor{envCfg: envCfg}, sshCfg: envCfg.SSH, projectRoot: "/p"}

	err := d.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssh.host")

	d.sshCfg.Host = "example.com"
	err = d.validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deploy_path")

	d.sshCfg.DeployPath = "/srv/shop"
	assert.NoError(t, d.validate())
}

func TestSSHDeployerHooksWithoutConfig(t *testing.T) {
	d := &SSHDeployer{shopCfg: &shop.Config{}}
	hooks := d.deploymentHooks()
	assert.Equal(t, shop.ConfigDeploymentHooks{}, hooks)
}

func TestSSHDeployerHooksWithConfig(t *testing.T) {
	d := &SSHDeployer{shopCfg: &shop.Config{
		ConfigDeployment: &shop.ConfigDeployment{
			Hooks: shop.ConfigDeploymentHooks{Pre: "echo pre", Post: "echo post"},
		},
	}}

	hooks := d.deploymentHooks()
	assert.Equal(t, "echo pre", hooks.Pre)
	assert.Equal(t, "echo post", hooks.Post)
}

func TestSSHDeployerShouldClearCacheDefault(t *testing.T) {
	d := &SSHDeployer{shopCfg: &shop.Config{}}
	assert.True(t, d.shouldClearCache())
}

func TestSSHDeployerShouldClearCacheConfigured(t *testing.T) {
	d := &SSHDeployer{shopCfg: &shop.Config{
		ConfigDeployment: &shop.ConfigDeployment{
			Cache: shop.ConfigDeploymentCache{AlwaysClear: false},
		},
	}}
	assert.False(t, d.shouldClearCache())
}

func TestExpandHome(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	assert.Equal(t, "/home/test/.ssh/id_rsa", expandHome("~/.ssh/id_rsa"))
	assert.Equal(t, "/abs/path", expandHome("/abs/path"))
	assert.Equal(t, "relative", expandHome("relative"))
}

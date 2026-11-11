package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHHostHooksInheritance(t *testing.T) {
	common := &EnvironmentSSHDeploymentHooks{TimeoutSeconds: 12, Pre: []string{"stop"}, Post: []string{"start"}}
	cfg := &EnvironmentSSHConfig{Directory: "/srv/shop/current", Hooks: common, MigrationHost: "a", Hosts: map[string]*EnvironmentSSHHostConfig{
		"a": {Host: "a.example"},
		"b": {Host: "b.example", Hooks: &EnvironmentSSHDeploymentHooks{}},
		"c": {Host: "c.example", Hooks: &EnvironmentSSHDeploymentHooks{Post: []string{"restart"}}},
	}}
	hosts, _, err := cfg.ResolveHosts()
	require.NoError(t, err)
	require.Len(t, hosts, 3)
	assert.Equal(t, common, hosts[0].Config.Hooks)
	assert.Empty(t, hosts[1].Config.Hooks.Pre)
	assert.Empty(t, hosts[1].Config.Hooks.Post)
	assert.Empty(t, hosts[2].Config.Hooks.Pre)
	assert.Equal(t, []string{"restart"}, hosts[2].Config.Hooks.Post)
}

func TestSSHHookTimeoutValidation(t *testing.T) {
	for _, timeout := range []int{-1, 0, 30} {
		cfg := &EnvironmentSSHConfig{Host: "example.com", Directory: "/srv/shop/current", Hooks: &EnvironmentSSHDeploymentHooks{TimeoutSeconds: timeout}}
		if timeout < 0 {
			assert.Error(t, cfg.Validate())
		} else {
			assert.NoError(t, cfg.Validate())
		}
	}
}

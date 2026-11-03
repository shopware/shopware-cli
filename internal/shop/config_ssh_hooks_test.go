package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHHostHooksInheritance(t *testing.T) {
	common := &EnvironmentSSHDeploymentHooks{Pre: []string{"stop"}, Post: []string{"start"}}
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

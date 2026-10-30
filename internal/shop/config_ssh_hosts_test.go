package shop

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSSHResolveHostsInheritanceAndIsolation checks host overrides and independence of mutable resolved settings.
func TestSSHResolveHostsInheritanceAndIsolation(t *testing.T) {
	base := &EnvironmentSSHConfig{
		User: "deploy", Port: 2222, Directory: "/srv/shop", IdentityFile: "/key", PHPBinary: "php8.4",
		PHPArgs:       []string{"-d", "memory_limit=192M"},
		MigrationHost: "web-2", Parallelism: 3,
		Shared: &EnvironmentSSHSharedConfig{Files: new([]string{"base"}), Directories: new([]string{"data"})},
		Hosts: map[string]*EnvironmentSSHHostConfig{
			"web-2": {Host: "two"},
			"web-1": {
				Host: "one", User: "other", Port: 23, Directory: "/other", IdentityFile: "/other-key", PHPBinary: "php",
				Shared: &EnvironmentSSHSharedConfig{Files: new([]string{})},
			},
		},
	}
	hosts, primary, err := base.ResolveHosts()
	require.NoError(t, err)
	require.Len(t, hosts, 2)
	assert.Equal(t, "web-2", primary)
	assert.Equal(t, "web-1", hosts[0].Name)
	one, two := hosts[0].Config, hosts[1].Config
	assert.Equal(t, "other", one.User)
	assert.Equal(t, 23, one.Port)
	assert.Equal(t, "/other", one.Directory)
	assert.Equal(t, "/other-key", one.IdentityFile)
	assert.Equal(t, "php", one.PHPBinary)
	assert.Nil(t, one.Shared.Directories)
	assert.Equal(t, "deploy", two.User)
	assert.Equal(t, 2222, two.Port)
	assert.Equal(t, "/srv/shop", two.Directory)
	assert.Equal(t, "/key", two.IdentityFile)
	assert.Equal(t, "php8.4", two.PHPBinary)
	for _, host := range hosts {
		assert.Equal(t, base.PHPArgs, host.Config.PHPArgs)
		assert.Nil(t, host.Config.Hosts)
		assert.Empty(t, host.Config.MigrationHost)
		assert.Zero(t, host.Config.Parallelism)
	}
	one.PHPArgs[1] = "memory_limit=256M"
	assert.Equal(t, []string{"-d", "memory_limit=192M"}, base.PHPArgs)
	assert.Equal(t, base.PHPArgs, two.PHPArgs)
	(*two.Shared.Files)[0] = "changed"
	(*two.Shared.Directories)[0] = "changed"
	*one.Shared.Files = append(*one.Shared.Files, "changed")
	assert.Equal(t, []string{"base"}, *base.Shared.Files)
	assert.Equal(t, []string{"data"}, *base.Shared.Directories)
	assert.Empty(t, *base.Hosts["web-1"].Shared.Files)
	assert.Equal(t, 3, base.Parallelism)
}

// TestSSHResolveHostsSingle checks resolution of single-host configurations.
func TestSSHResolveHostsSingle(t *testing.T) {
	legacy := &EnvironmentSSHConfig{Host: "example.com", Directory: "/srv/shop"}
	hosts, primary, err := legacy.ResolveHosts()
	require.NoError(t, err)
	assert.Empty(t, primary)
	require.Len(t, hosts, 1)
	assert.Empty(t, hosts[0].Name)
	assert.Equal(t, legacy, hosts[0].Config)
	assert.NotSame(t, legacy, hosts[0].Config)
	named := &EnvironmentSSHConfig{Directory: "/srv/shop", Hosts: map[string]*EnvironmentSSHHostConfig{"web": {Host: "example.com"}}}
	hosts, primary, err = named.ResolveHosts()
	require.NoError(t, err)
	assert.Equal(t, "web", primary)
	assert.Equal(t, legacy, hosts[0].Config)
}

// TestSSHResolveHostsInvalid checks rejection of ambiguous or invalid host groups.
func TestSSHResolveHostsInvalid(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *EnvironmentSSHConfig
		message string
	}{
		{"nil", nil, "requires ssh.host"},
		{"missing host", &EnvironmentSSHConfig{}, "requires ssh.host"},
		{"missing directory", &EnvironmentSSHConfig{Host: "one"}, "requires ssh.directory"},
		{"negative parallelism", &EnvironmentSSHConfig{Parallelism: -1}, "parallelism"},
		{"excessive parallelism", &EnvironmentSSHConfig{Parallelism: 33}, "parallelism"},
		{"both", &EnvironmentSSHConfig{Host: "one", Hosts: map[string]*EnvironmentSSHHostConfig{"web": {Host: "two"}}}, "mutually exclusive"},
		{"nil member", &EnvironmentSSHConfig{Hosts: map[string]*EnvironmentSSHHostConfig{"web": nil}}, "must not be null"},
		{"member missing host", &EnvironmentSSHConfig{Directory: "/srv", Hosts: map[string]*EnvironmentSSHHostConfig{"web": {}}}, "requires ssh.host"},
		{"member missing directory", &EnvironmentSSHConfig{Hosts: map[string]*EnvironmentSSHHostConfig{"web": {Host: "one"}}}, "requires ssh.directory"},
		{"unknown primary", &EnvironmentSSHConfig{MigrationHost: "other", Hosts: map[string]*EnvironmentSSHHostConfig{"web": {Host: "one"}}}, "must name an entry"},
		{"legacy primary", &EnvironmentSSHConfig{Host: "one", MigrationHost: "other"}, "must name an entry"},
		{"missing primary", &EnvironmentSSHConfig{Hosts: map[string]*EnvironmentSSHHostConfig{"a": {}, "b": {}}}, "required with multiple"},
		{"duplicate", &EnvironmentSSHConfig{Directory: "/srv", MigrationHost: "a", Hosts: map[string]*EnvironmentSSHHostConfig{"a": {Host: "one"}, "b": {Host: "one", Port: 22}}}, "same host"},
		{"bad shared path", &EnvironmentSSHConfig{Host: "one", Directory: "/srv", Shared: &EnvironmentSSHSharedConfig{Files: new([]string{"../escape"})}}, "project-relative"},
	}
	for _, label := range []string{"", "-web", "../web", "web/x", strings.Repeat("a", 129)} {
		tests = append(tests, struct {
			name    string
			cfg     *EnvironmentSSHConfig
			message string
		}{
			"label " + label, &EnvironmentSSHConfig{Directory: "/srv", Hosts: map[string]*EnvironmentSSHHostConfig{label: {Host: "one"}}}, "invalid ssh.hosts label",
		})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := tt.cfg.ResolveHosts()
			require.ErrorContains(t, err, tt.message)
		})
	}
}

package shop

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
)

// EnvironmentSSHHostConfig inherits unset scalar fields from the common SSH
// configuration. Shared and Cachetool replace the entire common block when set;
// adapter modes and shared lists are never merged with the common block.
type EnvironmentSSHHostConfig struct {
	Host         string `yaml:"host" jsonschema:"required"`
	User         string `yaml:"user,omitempty"`
	Port         int    `yaml:"port,omitempty"`
	Directory    string `yaml:"directory,omitempty"`
	IdentityFile string `yaml:"identity_file,omitempty"`
	PHPBinary    string `yaml:"php_binary,omitempty"`
	// Replaces the entire common shared block when supplied.
	Shared *EnvironmentSSHSharedConfig `yaml:"shared,omitempty"`
	// Replaces the entire common cachetool block when supplied.
	Cachetool *EnvironmentSSHCachetoolConfig `yaml:"cachetool,omitempty"`
}

type SSHHost struct {
	Name   string
	Config *EnvironmentSSHConfig
}

var sshHostLabel = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// ResolveHosts returns independent transport configurations in stable name order.
// It is intentionally not called while loading partial YAML includes.
func (c *EnvironmentSSHConfig) ResolveHosts() ([]SSHHost, string, error) {
	if c == nil {
		return nil, "", errors.New("ssh environment requires ssh.host")
	}
	if c.Parallelism < 0 || c.Parallelism > 32 {
		return nil, "", errors.New("ssh.parallelism must be between 0 and 32")
	}
	if c.Host != "" && len(c.Hosts) != 0 {
		return nil, "", errors.New("ssh.host and ssh.hosts are mutually exclusive")
	}
	if len(c.Hosts) == 0 {
		if c.MigrationHost != "" {
			return nil, "", errors.New("ssh.migration_host must name an entry in ssh.hosts")
		}
		member := cloneSSHMember(c)
		if err := validateSSHMember(member); err != nil {
			return nil, "", err
		}
		return []SSHHost{{Config: member}}, "", nil
	}
	names := make([]string, 0, len(c.Hosts))
	for name := range c.Hosts {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if !sshHostLabel.MatchString(name) {
			return nil, "", fmt.Errorf("invalid ssh.hosts label %q: use 1-128 letters, digits, dots, underscores or hyphens, starting with a letter or digit", name)
		}
	}
	primary := c.MigrationHost
	if primary == "" && len(names) == 1 {
		primary = names[0]
	}
	if primary == "" {
		return nil, "", errors.New("ssh.migration_host is required with multiple hosts")
	}
	if _, ok := c.Hosts[primary]; !ok {
		return nil, "", errors.New("ssh.migration_host must name an entry in ssh.hosts")
	}
	type endpoint struct {
		host, user, directory string
		port                  int
	}
	seen := make(map[endpoint]string)
	hosts := make([]SSHHost, 0, len(names))
	for _, name := range names {
		override := c.Hosts[name]
		if override == nil {
			return nil, "", fmt.Errorf("ssh.hosts.%s must not be null", name)
		}
		member := *c
		member.Host = override.Host
		for _, field := range []struct {
			target *string
			value  string
		}{
			{&member.User, override.User}, {&member.Directory, override.Directory},
			{&member.IdentityFile, override.IdentityFile}, {&member.PHPBinary, override.PHPBinary},
		} {
			if field.value != "" {
				*field.target = field.value
			}
		}
		if override.Port != 0 {
			member.Port = override.Port
		}
		if override.Shared != nil {
			member.Shared = override.Shared
		}
		if override.Cachetool != nil {
			member.Cachetool = override.Cachetool
		}
		resolved := cloneSSHMember(&member)
		if err := validateSSHMember(resolved); err != nil {
			return nil, "", fmt.Errorf("ssh.hosts.%s: %w", name, err)
		}
		port := resolved.Port
		if port == 0 {
			port = 22
		}
		key := endpoint{resolved.Host, resolved.User, resolved.Directory, port}
		if previous, ok := seen[key]; ok {
			return nil, "", fmt.Errorf("ssh.hosts.%s and ssh.hosts.%s target the same host, user, port and directory", previous, name)
		}
		seen[key] = name
		hosts = append(hosts, SSHHost{Name: name, Config: resolved})
	}
	return hosts, primary, nil
}

func validateSSHMember(c *EnvironmentSSHConfig) error {
	if c.Host == "" {
		return errors.New("ssh environment requires ssh.host")
	}
	if c.Directory == "" {
		return errors.New("ssh environment requires ssh.directory")
	}
	if err := c.Cachetool.Validate(); err != nil {
		return err
	}
	_, _, err := c.SharedPaths()
	return err
}

func cloneSSHMember(c *EnvironmentSSHConfig) *EnvironmentSSHConfig {
	member := *c
	member.Hosts, member.MigrationHost, member.Parallelism = nil, "", 0
	if c.Shared != nil {
		shared := *c.Shared
		if shared.Files != nil {
			shared.Files = new(slices.Clone(*shared.Files))
		}
		if shared.Directories != nil {
			shared.Directories = new(slices.Clone(*shared.Directories))
		}
		member.Shared = &shared
	}
	if c.Cachetool != nil {
		cachetool := *c.Cachetool
		if cachetool.Enabled != nil {
			cachetool.Enabled = new(*cachetool.Enabled)
		}
		member.Cachetool = &cachetool
	}
	return &member
}

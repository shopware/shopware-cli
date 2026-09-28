package shop

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
	"unicode"
)

// EnvironmentSSHCachetoolConfig configures an opt-in web/FPM OPcache reset.
// CLI OPcache is not supported because it is separate from web-serving processes.
type EnvironmentSSHCachetoolConfig struct {
	// Enable CacheTool during SSH deployment activation. Disabled by default.
	Enabled *bool `yaml:"enabled,omitempty" json:"enabled,omitempty" jsonschema:"default=false"`
	// Inline adapter: fcgi (PHP-FPM) or web. Required when enabled without config.
	Adapter string `yaml:"adapter,omitempty" json:"adapter,omitempty" jsonschema:"enum=fcgi,enum=web"`
	// FCGI socket or host:port connection string. Omit for CacheTool autodetection; fcgi adapter only.
	FCGI string `yaml:"fcgi,omitempty" json:"fcgi,omitempty"`
	// Absolute remote POSIX PHP-FPM chroot path; fcgi adapter only.
	FCGIChroot string `yaml:"fcgi_chroot,omitempty" json:"fcgi_chroot,omitempty"`
	// Absolute remote POSIX directory for CacheTool temporary files.
	TmpDir string `yaml:"tmp_dir,omitempty" json:"tmp_dir,omitempty"`
	// Absolute HTTP(S) URL for the web adapter, without credentials, query or fragment. Required for web.
	WebURL string `yaml:"web_url,omitempty" json:"web_url,omitempty" jsonschema:"format=uri"`
	// Absolute remote POSIX web document root; web adapter only. Defaults to the activated release/public.
	WebPath string `yaml:"web_path,omitempty" json:"web_path,omitempty"`
	// Absolute path to an existing remote native CacheTool YAML file targeting web/FPM, not CLI.
	// Mutually exclusive with all inline adapter fields, including tmp_dir, even when disabled.
	// With native web configuration, the owner must clean up temporary web endpoints if CacheTool fails to fetch them.
	Config string `yaml:"config,omitempty" json:"config,omitempty"`
}

func (c *EnvironmentSSHCachetoolConfig) IsEnabled() bool {
	return c != nil && c.Enabled != nil && *c.Enabled
}

// Validate checks supplied options even when disabled, so configuration mistakes
// are caught before deployment. Remote existence and native YAML are checked remotely.
func (c *EnvironmentSSHCachetoolConfig) Validate() error {
	if c == nil {
		return nil
	}
	if c.Adapter != "" && c.Adapter != "fcgi" && c.Adapter != "web" {
		return errors.New("ssh.cachetool.adapter must be fcgi or web")
	}
	if c.Config != "" && (c.Adapter != "" || c.FCGI != "" || c.FCGIChroot != "" || c.TmpDir != "" || c.WebURL != "" || c.WebPath != "") {
		return errors.New("ssh.cachetool.config cannot be combined with inline adapter options")
	}
	if c.IsEnabled() && c.Adapter == "" && c.Config == "" {
		return errors.New("ssh.cachetool requires adapter or config when enabled")
	}
	if (c.FCGI != "" || c.FCGIChroot != "") && c.Adapter != "fcgi" {
		return errors.New("ssh.cachetool.fcgi and fcgi_chroot require adapter fcgi")
	}
	if (c.WebURL != "" || c.WebPath != "") && c.Adapter != "web" {
		return errors.New("ssh.cachetool.web_url and web_path require adapter web")
	}
	if c.Adapter == "web" && c.WebURL == "" {
		return errors.New("ssh.cachetool.web_url is required for adapter web")
	}
	return c.validateLocations()
}

func (c *EnvironmentSSHCachetoolConfig) validateLocations() error {
	for _, field := range []struct{ name, value string }{
		{"fcgi_chroot", c.FCGIChroot}, {"tmp_dir", c.TmpDir},
		{"web_path", c.WebPath}, {"config", c.Config},
	} {
		if field.value != "" && (!path.IsAbs(field.value) ||
			strings.ContainsRune(field.value, '\\') ||
			strings.ContainsFunc(field.value, unicode.IsControl) ||
			strings.Contains("/"+field.value+"/", "/../")) {
			return fmt.Errorf("ssh.cachetool.%s must be an absolute remote POSIX path without parent traversal or control characters", field.name)
		}
	}
	if strings.ContainsFunc(c.FCGI, unicode.IsControl) {
		return errors.New("ssh.cachetool.fcgi must not contain control characters")
	}
	if c.WebURL != "" {
		u, err := url.Parse(c.WebURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") ||
			u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery ||
			strings.Contains(c.WebURL, "#") || strings.ContainsFunc(c.WebURL, unicode.IsControl) {
			return errors.New("ssh.cachetool.web_url must be an absolute HTTP(S) URL without credentials, query or fragment")
		}
	}
	return nil
}

func (c *Config) validateSSHCachetool() error {
	for name, env := range c.Environments {
		if env == nil || env.SSH == nil {
			continue
		}
		if err := env.SSH.Cachetool.Validate(); err != nil {
			return fmt.Errorf("environment %q: %w", name, err)
		}
		for hostName, host := range env.SSH.Hosts {
			if host != nil {
				if err := host.Cachetool.Validate(); err != nil {
					return fmt.Errorf("environment %q host %q: %w", name, hostName, err)
				}
			}
		}
	}
	return nil
}

// Mergo ignores pointed-to false values even with WithOverride. Preserve an
// explicit include override without letting an omitted setting disable a target.
func (c *Config) applyIncludedCachetoolEnabled(included *Config) {
	for name, env := range included.Environments {
		if env == nil || env.SSH == nil {
			continue
		}
		target := c.Environments[name]
		if target == nil || target.SSH == nil {
			continue
		}
		if env.SSH.Cachetool != nil && env.SSH.Cachetool.Enabled != nil && target.SSH.Cachetool != nil {
			target.SSH.Cachetool.Enabled = new(*env.SSH.Cachetool.Enabled)
		}
		for hostName, host := range env.SSH.Hosts {
			targetHost := target.SSH.Hosts[hostName]
			if host != nil && host.Cachetool != nil && host.Cachetool.Enabled != nil && targetHost != nil && targetHost.Cachetool != nil {
				targetHost.Cachetool.Enabled = new(*host.Cachetool.Enabled)
			}
		}
	}
}

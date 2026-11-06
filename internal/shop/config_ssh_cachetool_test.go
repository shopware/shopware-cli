package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHCachetoolValidate(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cfg   *EnvironmentSSHCachetoolConfig
		valid bool
	}{
		{"missing", nil, true},
		{"disabled", &EnvironmentSSHCachetoolConfig{}, true},
		{"fcgi", &EnvironmentSSHCachetoolConfig{Enabled: new(true), Adapter: "fcgi"}, true},
		{"web", &EnvironmentSSHCachetoolConfig{Adapter: "web", WebURL: "https://shop.example", WebPath: "/srv/public"}, true},
		{"native", &EnvironmentSSHCachetoolConfig{Enabled: new(true), Config: "/etc/cachetool.yml"}, true},
		{"missing target", &EnvironmentSSHCachetoolConfig{Enabled: new(true)}, false},
		{"cli", &EnvironmentSSHCachetoolConfig{Adapter: "cli"}, false},
		{"web missing path", &EnvironmentSSHCachetoolConfig{Adapter: "web", WebURL: "https://shop.example"}, false},
		{"web missing url", &EnvironmentSSHCachetoolConfig{Adapter: "web", WebPath: "/srv/public"}, false},
		{"web with fcgi", &EnvironmentSSHCachetoolConfig{Adapter: "web", FCGI: "/run/php.sock"}, false},
		{"fcgi with web", &EnvironmentSSHCachetoolConfig{Adapter: "fcgi", WebPath: "/srv/public"}, false},
		{"unscoped fcgi", &EnvironmentSSHCachetoolConfig{FCGI: "/run/php.sock"}, false},
		{"unscoped web", &EnvironmentSSHCachetoolConfig{WebURL: "https://shop.example"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.valid {
				require.NoError(t, tc.cfg.Validate())
			} else {
				require.Error(t, tc.cfg.Validate())
			}
		})
	}
}

func TestSSHCachetoolNativeConfigExcludesInlineOptions(t *testing.T) {
	for _, cfg := range []EnvironmentSSHCachetoolConfig{
		{Adapter: "fcgi"}, {FCGI: "/run/php.sock"}, {FCGIChroot: "/srv"},
		{TmpDir: "/tmp"}, {WebURL: "https://shop.example"}, {WebPath: "/srv/public"},
	} {
		cfg.Config = "/etc/cachetool.yml"
		require.Error(t, cfg.Validate())
	}
}

func TestSSHCachetoolInvalidPaths(t *testing.T) {
	for _, value := range []string{"relative", "../outside", "/srv/../etc", "/srv/..", "C:/remote", `/srv\remote`, "/srv/\x00", "/srv/\n", "/srv/\t"} {
		for _, field := range []string{"fcgi_chroot", "tmp_dir", "web_path", "config"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				cfg := &EnvironmentSSHCachetoolConfig{}
				switch field {
				case "fcgi_chroot":
					cfg.Adapter, cfg.FCGIChroot = "fcgi", value
				case "tmp_dir":
					cfg.TmpDir = value
				case "web_path":
					cfg.Adapter, cfg.WebURL, cfg.WebPath = "web", "https://shop.example", value
				case "config":
					cfg.Config = value
				}
				require.ErrorContains(t, cfg.Validate(), "ssh.cachetool."+field)
			})
		}
	}
}

func TestSSHCachetoolInvalidURLs(t *testing.T) {
	for _, value := range []string{"relative", "//shop.example", "ftp://shop.example", "https:///path", "https://user:secret@shop.example", "https://shop.example?q=1", "https://shop.example?", "https://shop.example#fragment", "https://shop.example#", "https://shop.example/\n", "https://shop.example/%zz"} {
		t.Run(value, func(t *testing.T) {
			cfg := &EnvironmentSSHCachetoolConfig{Adapter: "web", WebURL: value, WebPath: "/srv/cachetool"}
			require.ErrorContains(t, cfg.Validate(), "ssh.cachetool.web_url")
		})
	}
}

func TestSSHCachetoolIncludeEnabledOverride(t *testing.T) {
	for _, tc := range []struct {
		name        string
		enabled     *bool
		wantEnabled bool
	}{
		{"explicit false disables", new(false), false},
		{"omitted flag inherits", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &Config{Environments: map[string]*EnvironmentConfig{
				"production": {SSH: &EnvironmentSSHConfig{Cachetool: &EnvironmentSSHCachetoolConfig{
					Enabled: new(true), Adapter: "fcgi",
				}}},
			}}
			included := &Config{Environments: map[string]*EnvironmentConfig{
				"production": {SSH: &EnvironmentSSHConfig{Cachetool: &EnvironmentSSHCachetoolConfig{
					Enabled: tc.enabled,
				}}},
			}}
			config.applyIncludedCachetoolEnabled(included)
			assert.Equal(t, tc.wantEnabled, config.Environments["production"].SSH.Cachetool.IsEnabled())
		})
	}
}

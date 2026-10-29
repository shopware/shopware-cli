package shop

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
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

func TestSSHCachetoolLocalOnlyConfigValidated(t *testing.T) {
	file := filepath.Join(t.TempDir(), ".shopware-project.yml")
	testhelper.WriteFile(t, LocalConfigFileName(file), "environments:\n  production:\n    ssh:\n      cachetool: {enabled: true}\n")
	_, err := ReadConfig(t.Context(), file, true)
	require.ErrorContains(t, err, "ssh.cachetool")
}

func TestSSHCachetoolValidatesAfterIncludesAreMerged(t *testing.T) {
	for _, tc := range []struct {
		name, base, included string
	}{
		{"adapter supplied by base", "{adapter: fcgi}", "{enabled: true}"},
		{"adapter supplied by include", "{enabled: true}", "{adapter: fcgi}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			file := filepath.Join(dir, ".shopware-project.yml")
			testhelper.WriteFile(t, file, fmt.Sprintf(`
compatibility_date: "2026-01-01"
include: [included.yml]
environments:
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: /srv/shop/current
      cachetool: %s
`, tc.base))
			testhelper.WriteFile(t, filepath.Join(dir, "included.yml"), fmt.Sprintf(`
environments:
  production:
    ssh:
      cachetool: %s
`, tc.included))
			cfg, err := ReadConfig(t.Context(), file, false)
			require.NoError(t, err)
			assert.True(t, cfg.Environments["production"].SSH.Cachetool.IsEnabled())
			assert.Equal(t, "fcgi", cfg.Environments["production"].SSH.Cachetool.Adapter)
		})
	}
}

func TestSSHCachetoolIncludeCanDisableReset(t *testing.T) {
	for _, tc := range []struct {
		name, override string
		wantEnabled    bool
	}{
		{"explicit false disables", "{enabled: false}", false},
		{"omitted flag inherits", "{fcgi: /run/php/other.sock}", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			base := "compatibility_date: '2026-01-01'\ninclude: [override.yml]\nenvironments:\n  production:\n    ssh:\n      host: example.invalid\n      directory: /srv/shop/current\n      cachetool: {enabled: true, adapter: fcgi}\n"
			testhelper.WriteFile(t, filepath.Join(dir, ".shopware-project.yml"), base)
			testhelper.WriteFile(t, filepath.Join(dir, "override.yml"), "environments:\n  production:\n    ssh:\n      cachetool: "+tc.override+"\n")
			cfg, err := ReadConfig(t.Context(), filepath.Join(dir, ".shopware-project.yml"), false)
			require.NoError(t, err)
			assert.Equal(t, tc.wantEnabled, cfg.Environments["production"].SSH.Cachetool.IsEnabled())
			assert.Equal(t, "fcgi", cfg.Environments["production"].SSH.Cachetool.Adapter)
		})
	}
}

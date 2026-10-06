package shop

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSSHCachetoolReadConfig(t *testing.T) {
	for _, tc := range []struct {
		name, options string
		valid         bool
	}{
		{"missing", "", true},
		{"disabled", "{}", true},
		{"explicitly disabled", "{enabled: false}", true},
		{"fcgi autodetection", "{enabled: true, adapter: fcgi}", true},
		{"fcgi socket", "{enabled: true, adapter: fcgi, fcgi: /run/php/fpm.sock, fcgi_chroot: /srv/shop, tmp_dir: /tmp/cachetool}", true},
		{"fcgi tcp", "{enabled: true, adapter: fcgi, fcgi: '127.0.0.1:9000'}", true},
		{"web default path", "{enabled: true, adapter: web, web_url: 'https://shop.example'}", true},
		{"web custom path", "{enabled: true, adapter: web, web_url: 'http://localhost:8080/shop', web_path: /srv/shop/public}", true},
		{"native", "{enabled: true, config: /etc/cachetool.yml}", true},
		{"native disabled", "{config: /etc/cachetool.yml}", true},
		{"enabled missing target", "{enabled: true}", false},
		{"cli", "{adapter: cli}", false},
		{"unknown adapter disabled", "{adapter: invalid}", false},
		{"web missing url", "{enabled: true, adapter: web}", false},
		{"web fcgi", "{adapter: web, web_url: 'https://shop.example', fcgi: /run/php.sock}", false},
		{"fcgi web", "{adapter: fcgi, web_path: /srv/public}", false},
		{"unscoped fcgi", "{fcgi: /run/php.sock}", false},
		{"unscoped web", "{web_url: 'https://shop.example'}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), ".shopware-project.yml")
			testhelper.WriteFile(t, file, fmt.Sprintf("compatibility_date: '2026-01-01'\nenvironments:\n  production:\n    type: ssh\n    ssh:\n      host: example.invalid\n      directory: /srv/shop\n      cachetool: %s\n", tc.options))
			cfg, err := ReadConfig(t.Context(), file, false)
			if !tc.valid {
				require.ErrorContains(t, err, "ssh.cachetool")
				return
			}
			require.NoError(t, err)
			require.NoError(t, cfg.Environments["production"].SSH.Cachetool.Validate())
		})
	}
}

func TestSSHCachetoolNativeConfigExcludesEveryInlineOption(t *testing.T) {
	for _, option := range []string{"adapter: fcgi", "fcgi: /run/php.sock", "fcgi_chroot: /srv", "tmp_dir: /tmp", "web_url: 'https://shop.example'", "web_path: /srv/public"} {
		for _, enabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/enabled=%t", option, enabled), func(t *testing.T) {
				file := filepath.Join(t.TempDir(), ".shopware-project.yml")
				testhelper.WriteFile(t, file, fmt.Sprintf("environments:\n  production:\n    ssh:\n      cachetool: {enabled: %t, config: /etc/cachetool.yml, %s}\n", enabled, option))
				_, err := ReadConfig(t.Context(), file, false)
				require.ErrorContains(t, err, "ssh.cachetool.config cannot be combined")
			})
		}
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
			cfg := &EnvironmentSSHCachetoolConfig{Adapter: "web", WebURL: value}
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

func TestSSHCachetoolJSONNames(t *testing.T) {
	cfg := &EnvironmentSSHCachetoolConfig{Enabled: new(true), Adapter: "fcgi", FCGI: "/run/php.sock", FCGIChroot: "/srv", TmpDir: "/tmp", WebURL: "https://shop.example", WebPath: "/srv/public", Config: "/etc/cachetool.yml"}
	data, err := json.Marshal(cfg)
	require.NoError(t, err)
	assert.JSONEq(t, `{"enabled":true,"adapter":"fcgi","fcgi":"/run/php.sock","fcgi_chroot":"/srv","tmp_dir":"/tmp","web_url":"https://shop.example","web_path":"/srv/public","config":"/etc/cachetool.yml"}`, string(data))
	data, err = json.Marshal(&EnvironmentSSHCachetoolConfig{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(data))
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
			base := "compatibility_date: '2026-01-01'\ninclude: [override.yml]\nenvironments:\n  production:\n    ssh:\n      cachetool: {enabled: true, adapter: fcgi}\n"
			testhelper.WriteFile(t, filepath.Join(dir, ".shopware-project.yml"), base)
			testhelper.WriteFile(t, filepath.Join(dir, "override.yml"), "environments:\n  production:\n    ssh:\n      cachetool: "+tc.override+"\n")
			cfg, err := ReadConfig(t.Context(), filepath.Join(dir, ".shopware-project.yml"), false)
			require.NoError(t, err)
			assert.Equal(t, tc.wantEnabled, cfg.Environments["production"].SSH.Cachetool.IsEnabled())
			assert.Equal(t, "fcgi", cfg.Environments["production"].SSH.Cachetool.Adapter)
		})
	}
}

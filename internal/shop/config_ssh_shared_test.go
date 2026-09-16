package shop

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSSHSharedPaths(t *testing.T) {
	var omitted *EnvironmentSSHConfig
	defaultFiles, defaultDirectories, err := omitted.SharedPaths()
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{".env.local", "install.lock", "public/.htaccess", "public/.user.ini"}, defaultFiles)
	assert.ElementsMatch(t, []string{"config/jwt", "files", "var/log", "public/media", "public/plugins", "public/thumbnail", "public/sitemap", "public/theme"}, defaultDirectories)

	for _, tc := range []struct {
		name, config       string
		files, directories []string
	}{
		{"omitted", "{}", defaultFiles, defaultDirectories},
		{"empty mapping", "shared: {}", defaultFiles, defaultDirectories},
		{"replace files", "shared:\n  files: [custom/runtime.ini]", []string{"custom/runtime.ini"}, defaultDirectories},
		{"replace directories", "shared:\n  directories: [custom/data]", defaultFiles, []string{"custom/data"}},
		{"disable files", "shared:\n  files: []", []string{}, defaultDirectories},
		{"disable directories", "shared:\n  directories: []", defaultFiles, []string{}},
		{"disable both", "shared:\n  files: []\n  directories: []", []string{}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg EnvironmentSSHConfig
			require.NoError(t, yaml.Unmarshal([]byte(tc.config), &cfg))
			for range 2 {
				files, directories, err := cfg.SharedPaths()
				require.NoError(t, err)
				assert.ElementsMatch(t, tc.files, files)
				assert.ElementsMatch(t, tc.directories, directories)
				encoded, err := yaml.Marshal(cfg)
				require.NoError(t, err)
				cfg = EnvironmentSSHConfig{}
				require.NoError(t, yaml.Unmarshal(encoded, &cfg), "explicit empty lists must survive serialization")
			}
		})
	}
}

func TestSSHSharedPathsRejectUnsafeAndOverlappingPaths(t *testing.T) {
	for _, name := range []string{"", ".", "..", "../outside", "/absolute", "public/../media", "./media", "media/", "public//media", `public\media`, "media\nother", "media\x00"} {
		t.Run(name, func(t *testing.T) {
			files := []string{name}
			cfg := &EnvironmentSSHConfig{Shared: &EnvironmentSSHSharedConfig{Files: &files}}
			_, _, err := cfg.SharedPaths()
			require.ErrorContains(t, err, "project-relative")
		})
	}
	for _, tc := range []struct{ files, directories []string }{
		{[]string{"same", "same"}, []string{}},
		{[]string{"parent", "parent/child"}, []string{}},
		{[]string{}, []string{"parent/child", "parent"}},
		{[]string{"public/custom"}, []string{"public"}},
		{[]string{"same"}, []string{"same"}},
	} {
		cfg := &EnvironmentSSHConfig{Shared: &EnvironmentSSHSharedConfig{Files: &tc.files, Directories: &tc.directories}}
		_, _, err := cfg.SharedPaths()
		require.ErrorContains(t, err, "overlap")
	}
}

func TestSSHSharedPathsLocalOverrideAndWriteConfig(t *testing.T) {
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-project.yml"), `
compatibility_date: "2026-01-01"
environments:
  production:
    type: ssh
    ssh:
      host: example.invalid
      directory: /var/www/shop/current
      shared:
        files: [.env.local, install.lock]
        directories: [custom/data]
`)
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-project.local.yml"), `
environments:
  production:
    ssh:
      shared:
        files: !override []
`)
	cfg, err := ReadConfig(t.Context(), filepath.Join(root, ".shopware-project.yml"), true)
	require.NoError(t, err)
	for range 2 {
		env, err := cfg.ResolveEnvironment("production")
		require.NoError(t, err)
		files, directories, err := env.SSH.SharedPaths()
		require.NoError(t, err)
		assert.Empty(t, files)
		assert.ElementsMatch(t, []string{"custom/data"}, directories)
		output := t.TempDir()
		require.NoError(t, WriteConfig(cfg, output))
		cfg, err = ReadConfig(t.Context(), filepath.Join(output, ".shopware-project.yml"), true)
		require.NoError(t, err)
	}
}

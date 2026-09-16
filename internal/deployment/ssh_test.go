package deployment

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSSHCreateBuildsLocallyAndRetainsArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a PHP shell stand-in")
	}
	root := t.TempDir()
	bin := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(bin, "php"), "#!/bin/sh\n[ \"$1\" = bin/ci ] || exit 1\nprintf built > generated.txt\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "php"), 0o755))
	testhelper.WriteFile(t, filepath.Join(bin, "ssh"), "#!/bin/sh\nexit 99\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "ssh"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PHP_BINARY", "")
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), `{"require":{"shopware/core":"6.7.0.0"}}`)
	testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php")
	configPath := filepath.Join(root, ".shopware-project.yml")
	testhelper.WriteFile(t, configPath, `
compatibility_date: "2026-01-01"
disable_composer_install: true
build:
  disable_checksums: true
`)
	cfg, err := shop.ReadConfig(t.Context(), configPath, true)
	require.NoError(t, err)
	env := &shop.EnvironmentConfig{Type: "ssh", SSH: &shop.EnvironmentSSHConfig{Host: "example.invalid", Directory: "/var/www/shop/current"}}
	target, err := executor.New(root, env, cfg)
	require.NoError(t, err)
	backend := NewSSH(target.(*executor.SSHExecutor), root, cfg, env, projectbuild.ArchiveOptions{ConfigPath: configPath})
	first, err := backend.CreateDeployment(t.Context())
	require.NoError(t, err)
	second, err := backend.CreateDeployment(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, first.Reference, second.Reference)
	assert.FileExists(t, first.Reference)
	assert.FileExists(t, second.Reference)
	assert.Contains(t, first.Reference, filepath.Join(".shopware-cli", "deployments"))
	assert.NoFileExists(t, filepath.Join(root, "generated.txt"), "build must not modify the source project")
}

package deployment

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestNewBackend(t *testing.T) {
	for _, kind := range []string{"", "local", "docker", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			_, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{Type: kind})
			require.ErrorIs(t, err, ErrNotSupported)
		})
	}
	_, err := New(t.TempDir(), "", &shop.Config{}, nil)
	require.ErrorIs(t, err, ErrNotSupported)
	for _, config := range []*shop.EnvironmentSSHConfig{
		nil, {}, {Host: "example.invalid"}, {Directory: "/srv/shop"},
	} {
		_, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{Type: "ssh", SSH: config})
		require.Error(t, err)
	}
	backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh", SSH: &shop.EnvironmentSSHConfig{Host: "example.invalid", Directory: "/srv/shop"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ssh", backend.Type())
	_, generalExecutor := backend.(executor.Executor)
	assert.False(t, generalExecutor, "deployment backends must not expose general execution")
}

func TestSSHCreateBuildsLocallyAndRetainsArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a PHP shell stand-in")
	}
	root := t.TempDir()
	bin := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(bin, "php"), "#!/bin/sh\n[ \"$1\" = bin/ci ] || exit 1\nprintf built > generated.txt\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "php"), 0o755))
	testhelper.WriteFile(t, filepath.Join(bin, "ssh"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$SSH_ARGS_LOG\"\nexit 99\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "ssh"), 0o755))
	sshArgsLog := filepath.Join(bin, "ssh-args.log")
	t.Setenv("SSH_ARGS_LOG", sshArgsLog)
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
	backend, err := New(root, configPath, cfg, env)
	require.NoError(t, err)
	first, err := backend.CreateDeployment(t.Context(), CreateOptions{})
	require.NoError(t, err)
	second, err := backend.CreateDeployment(t.Context(), CreateOptions{})
	require.NoError(t, err)
	assert.NotEqual(t, first.Reference, second.Reference)
	assert.Regexp(t, `^[a-z]+-[a-z]+-[a-z]+$`, first.Reference)
	assert.Regexp(t, `^[a-z]+-[a-z]+-[a-z]+$`, second.Reference)
	assert.FileExists(t, filepath.Join(root, ".shopware-cli", "deployments", first.Reference+".tar.gz"))
	assert.FileExists(t, filepath.Join(root, ".shopware-cli", "deployments", second.Reference+".tar.gz"))
	assert.NoFileExists(t, sshArgsLog, "creation must not open SSH")
	customPath := filepath.Join(t.TempDir(), "custom.tar.gz")
	customDeployment, err := backend.CreateDeployment(t.Context(), CreateOptions{OutputPath: customPath})
	require.NoError(t, err)
	assert.Equal(t, customPath, customDeployment.Reference)
	assert.FileExists(t, customPath)
	before, err := os.ReadFile(customPath)
	require.NoError(t, err)
	_, err = backend.CreateDeployment(t.Context(), CreateOptions{OutputPath: customPath})
	require.Error(t, err)
	after, err := os.ReadFile(customPath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "existing deployments must not be overwritten")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	canceledPath := filepath.Join(t.TempDir(), "canceled.tar.gz")
	artifact, err := backend.CreateDeployment(ctx, CreateOptions{OutputPath: canceledPath})
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, artifact.Reference)
	assert.NoFileExists(t, canceledPath)
	assert.NoFileExists(t, sshArgsLog)
	assert.NoFileExists(t, filepath.Join(root, "generated.txt"), "build must not modify the source project")
}

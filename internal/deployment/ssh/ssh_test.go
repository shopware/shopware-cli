package ssh

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
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
	first, err := backend.CreateDeployment(t.Context(), deployment.CreateOptions{})
	require.NoError(t, err)
	second, err := backend.CreateDeployment(t.Context(), deployment.CreateOptions{})
	require.NoError(t, err)
	assert.NotEqual(t, first.Reference, second.Reference)
	assert.Regexp(t, `^[a-z]+-[a-z]+-[a-z]+$`, first.Reference)
	assert.Regexp(t, `^[a-z]+-[a-z]+-[a-z]+$`, second.Reference)
	assert.FileExists(t, filepath.Join(root, ".shopware-cli", "deployments", first.Reference+".tar.gz"))
	assert.FileExists(t, filepath.Join(root, ".shopware-cli", "deployments", second.Reference+".tar.gz"))
	assert.NoFileExists(t, sshArgsLog, "creation must not open SSH")
	customPath := filepath.Join(t.TempDir(), "custom.tar.gz")
	customDeployment, err := backend.CreateDeployment(t.Context(), deployment.CreateOptions{OutputPath: customPath})
	require.NoError(t, err)
	assert.Equal(t, customPath, customDeployment.Reference)
	assert.FileExists(t, customPath)
	before, err := os.ReadFile(customPath)
	require.NoError(t, err)
	_, err = backend.CreateDeployment(t.Context(), deployment.CreateOptions{OutputPath: customPath})
	require.Error(t, err)
	after, err := os.ReadFile(customPath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "existing deployments must not be overwritten")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	canceledPath := filepath.Join(t.TempDir(), "canceled.tar.gz")
	artifact, err := backend.CreateDeployment(ctx, deployment.CreateOptions{OutputPath: canceledPath})
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, artifact.Reference)
	assert.NoFileExists(t, canceledPath)
	assert.NoFileExists(t, sshArgsLog)
	assert.NoFileExists(t, filepath.Join(root, "generated.txt"), "build must not modify the source project")
}

func TestSSHInitializationValues(t *testing.T) {
	assert.NoError(t, validateDatabaseURL("mysql://user:password@db.example.com:3306/shopware"))
	assert.NoError(t, validateDatabaseURL("mariadb://user@db.example.com/shopware?ssl=true"))
	assert.Error(t, validateDatabaseURL("postgres://user@db.example.com/shopware"))
	assert.Error(t, validateDatabaseURL("mysql://db.example.com"))
	assert.Equal(t, "mysql://us%40er:p%40ss%2Fword@db.example.com:3307/shop%20name",
		buildDatabaseURL("db.example.com", "3307", "shop name", "us@er", "p@ss/word"))
	secret, err := deploymentSecret()
	require.NoError(t, err)
	assert.Len(t, secret, 64)
}

func TestSSHInitializationWithoutSharedEnvLocal(t *testing.T) {
	backend := localDeploymentSSH(t)
	backend.env.SSH.Shared = &shop.EnvironmentSSHSharedConfig{Files: new([]string{})}
	called := false
	err := backend.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		called = true
		return sshDeploymentInitConfig{RuntimeValues: map[string]string{"APP_ENV": "prod"}}, true, nil
	})
	require.NoError(t, err)
	assert.True(t, called)
}

func TestSSHInitializationCancellationDoesNotWrite(t *testing.T) {
	store := &memoryInitializationStore{}
	backend := &SSH{initStore: store}
	err := backend.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		return sshDeploymentInitConfig{RuntimeValues: map[string]string{"APP_SECRET": "not-written"}}, false, nil
	})
	require.ErrorIs(t, err, deployment.ErrInitializationCanceled)
	assert.Empty(t, store.writes)
	assert.Equal(t, 1, store.inspections)
}

func TestSSHInitializationRepeatDoesNotPromptOrWrite(t *testing.T) {
	store := &memoryInitializationStore{}
	store.states[0].Initialized = true
	backend := &SSH{initStore: store}
	err := backend.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		t.Fatal("completed initialization must not prompt")
		return sshDeploymentInitConfig{}, false, nil
	})
	require.NoError(t, err)
	assert.Empty(t, store.writes)
	assert.Equal(t, 1, store.inspections)
}

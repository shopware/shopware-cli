package deployment

import (
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
			assert.Contains(t, err.Error(), "deployments are not supported for environment type")
		})
	}
	_, err := New(t.TempDir(), "", &shop.Config{}, nil)
	require.ErrorIs(t, err, ErrNotSupported)
	assert.Contains(t, err.Error(), `environment type "local"`)
	_, err = New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{Type: "ssh"})
	require.ErrorContains(t, err, "requires an ssh section")
	backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh", SSH: &shop.EnvironmentSSHConfig{Host: "example.invalid", Directory: "/srv/shop/current"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ssh", backend.Type())
	_, generalExecutor := backend.(executor.Executor)
	assert.False(t, generalExecutor, "deployment backends must not expose general execution")
}

func TestSSHInitializationValues(t *testing.T) {
	assert.NoError(t, validateApplicationURL("https://shop.example.com"))
	assert.Error(t, validateApplicationURL("shop.example.com"))
	assert.Error(t, validateApplicationURL("https://user:secret@shop.example.com"))
	assert.NoError(t, validateOptionalApplicationURL(""))
	assert.NoError(t, validateDatabaseURL("mysql://user:password@db.example.com:3306/shopware"))
	assert.NoError(t, validateDatabaseURL("mariadb://user@db.example.com/shopware?ssl=true"))
	assert.Error(t, validateDatabaseURL("postgres://user@db.example.com/shopware"))
	assert.Error(t, validateDatabaseURL("mysql://db.example.com"))
	assert.NoError(t, validateAdminPassword("long-enough"))
	assert.Error(t, validateAdminPassword("short"))
	assert.Error(t, validateAdminPassword("long-enough\nbut-multiline"))
	assert.NoError(t, validateOptionalEmail(""))
	assert.NoError(t, validateOptionalEmail("admin@example.com"))
	assert.Error(t, validateOptionalEmail("Admin <admin@example.com>"))
	assert.Equal(t, "mysql://us%40er:p%40ss%2Fword@db.example.com:3307/shop%20name",
		buildDatabaseURL("db.example.com", "3307", "shop name", "us@er", "p@ss/word"))
	secret, err := deploymentSecret()
	require.NoError(t, err)
	assert.Len(t, secret, 64)
}

func TestSSHInitializationRequiresSharedEnvLocal(t *testing.T) {
	files := []string{"custom/runtime.ini"}
	env := &shop.EnvironmentConfig{Type: executor.TypeSSH, SSH: &shop.EnvironmentSSHConfig{
		Host: "example.invalid", Directory: "/var/www/shop/current",
		Shared: &shop.EnvironmentSSHSharedConfig{Files: &files},
	}}
	backend, err := New(t.TempDir(), "", &shop.Config{}, env)
	require.NoError(t, err)
	require.ErrorContains(t, backend.(Initializer).InitializeDeployment(t.Context()), `requires ".env.local"`)
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
	_, err = backend.RolloutDeployment(t.Context(), first, nil)
	require.ErrorContains(t, err, "SSH rollout", "the generated name must resolve before invoking SSH")
	sshArgs, err := os.ReadFile(sshArgsLog)
	require.NoError(t, err)
	assert.Contains(t, string(sshArgs), `"deployment":"`+first.Reference+`"`, "persist the original reference, not the resolved archive path")
	customPath := filepath.Join(t.TempDir(), "custom.tar.gz")
	customDeployment, err := backend.CreateDeployment(t.Context(), CreateOptions{OutputPath: customPath})
	require.NoError(t, err)
	assert.Equal(t, customPath, customDeployment.Reference)
	assert.FileExists(t, customPath)
	assert.NoFileExists(t, filepath.Join(root, "generated.txt"), "build must not modify the source project")
}

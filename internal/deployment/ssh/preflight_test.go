package ssh

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func preflightBackend(t *testing.T) *SSH {
	t.Helper()
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/deployments/release.tar.gz"), "archive")
	remote := filepath.Join(t.TempDir(), "not-created", "live")
	backend, err := New(root, filepath.Join(root, ".config/shopware-project.yml"), &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh",
		AdminApi: &shop.ConfigAdminApi{
			ClientSecret: "never-print-client-secret",
			Password:     "never-print-admin-password",
		},
		SSH: &shop.EnvironmentSSHConfig{
			Host: "production-alias", User: "deploy", Port: 2222, Directory: remote,
			IdentityFile: "/private/identity-key",
		},
	})
	require.NoError(t, err)
	// A preview must not resolve binaries, open SSH or prepare the target.
	t.Setenv("PATH", t.TempDir())
	return backend.(*SSH)
}

func TestSSHDeploymentPreflightResolvesLocalPaths(t *testing.T) {
	s := preflightBackend(t)
	cwd := filepath.Join(s.root, "subdirectory")
	require.NoError(t, os.Mkdir(cwd, 0o755))
	t.Chdir(cwd)
	s.configPath = "../custom-project.yml"
	for _, reference := range []string{"release", "../.shopware-cli/deployments/release.tar.gz"} {
		preview, err := s.DeploymentPreflight(t.Context(), Deployment{Reference: reference}, PreflightRollout)
		require.NoError(t, err)
		assert.Equal(t, s.root, preview.ProjectDirectory)
		assert.Equal(t, filepath.Join(s.root, "custom-project.yml"), preview.ConfigPath)
		assert.Equal(t, filepath.Join(s.root, ".shopware-cli/deployments/release.tar.gz"), preview.ArchivePath)
		assert.Equal(t, []PreflightTarget{{
			Host: "production-alias", User: "deploy", Port: 2222,
			Directory: s.directory, DeploymentRoot: filepath.Dir(s.directory), Migration: true,
		}}, preview.Targets)
	}
	assert.NoDirExists(t, filepath.Dir(s.directory))
}

func TestSSHDeploymentPreflightRollbackNeedsNoLocalArchive(t *testing.T) {
	s := preflightBackend(t)
	require.NoError(t, os.RemoveAll(filepath.Join(s.root, ".shopware-cli")))
	preview, err := s.DeploymentPreflight(t.Context(), Deployment{Reference: "retained-release"}, PreflightRollback)
	require.NoError(t, err)
	assert.Empty(t, preview.ArchivePath)
	require.Len(t, preview.Targets, 1)
	assert.False(t, preview.Targets[0].Migration)
	_, err = s.DeploymentPreflight(t.Context(), Deployment{Reference: "../retained-release"}, PreflightRollback)
	require.Error(t, err)
	assert.NoDirExists(t, filepath.Dir(s.directory))
}

func TestSSHDeploymentPreflightShowsInheritedGroupTargets(t *testing.T) {
	s := preflightBackend(t)
	config := &shop.EnvironmentSSHConfig{
		User: "deploy", Port: 2222, Directory: "/srv/shop/live", MigrationHost: "web-b",
		Hosts: map[string]*shop.EnvironmentSSHHostConfig{
			"web-b": {Host: "second"},
			"web-a": {Host: "first", User: "release", Port: 2200, Directory: "/srv/other/shop"},
		},
	}
	backend, err := New(s.root, s.configPath, s.config, &shop.EnvironmentConfig{Type: "ssh", SSH: config})
	require.NoError(t, err)
	preview, err := backend.(PreflightProvider).DeploymentPreflight(t.Context(), Deployment{Reference: "release"}, PreflightRollout)
	require.NoError(t, err)
	assert.Equal(t, []PreflightTarget{
		{Name: "web-a", Host: "first", User: "release", Port: 2200, Directory: "/srv/other/shop", DeploymentRoot: "/srv/other"},
		{Name: "web-b", Host: "second", User: "deploy", Port: 2222, Directory: "/srv/shop/live", DeploymentRoot: "/srv/shop", Migration: true},
	}, preview.Targets)

	preview, err = backend.(PreflightProvider).DeploymentPreflight(t.Context(), Deployment{Reference: "release"}, PreflightRollback)
	require.NoError(t, err)
	for _, target := range preview.Targets {
		assert.False(t, target.Migration)
	}

	// A single named target must retain its name even without a group backend.
	delete(config.Hosts, "web-a")
	backend, err = New(s.root, s.configPath, s.config, &shop.EnvironmentConfig{Type: "ssh", SSH: config})
	require.NoError(t, err)
	preview, err = backend.(PreflightProvider).DeploymentPreflight(t.Context(), Deployment{Reference: "release"}, PreflightRollout)
	require.NoError(t, err)
	require.Len(t, preview.Targets, 1)
	assert.Equal(t, "web-b", preview.Targets[0].Name)
}

func TestWriteDeploymentPreflight(t *testing.T) {
	s := preflightBackend(t)
	t.Chdir(s.root)
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for _, environment := range []string{"", "production"} {
		var output bytes.Buffer
		require.NoError(t, WritePreflight(t.Context(), s, PreflightOptions{
			Action: PreflightRollout, Environment: environment, Deployment: Deployment{Reference: "release"},
		}, &output))
		for _, value := range []string{
			cwd, s.root, s.configPath, "release",
			filepath.Join(s.root, ".shopware-cli/deployments/release.tar.gz"),
			"production-alias", "deploy", "2222", s.directory, filepath.Dir(s.directory),
		} {
			assert.Contains(t, output.String(), value)
		}
		if environment == "" {
			environment = "local"
		}
		assert.Contains(t, output.String(), environment)
		for _, secret := range []string{"never-print-client-secret", "never-print-admin-password", "/private/identity-key"} {
			assert.NotContains(t, output.String(), secret)
		}
	}
	assert.NoDirExists(t, filepath.Dir(s.directory))

	s.env.SSH.User = ""
	s.env.SSH.Port = 0
	var output bytes.Buffer
	require.NoError(t, WritePreflight(t.Context(), s, PreflightOptions{
		Action: PreflightRollback, Deployment: Deployment{Reference: "retained"},
	}, &output))
	assert.Contains(t, output.String(), "SSH user: SSH config/default")
	assert.Contains(t, output.String(), "SSH port: SSH config/default")
	assert.NotContains(t, output.String(), "Archive:")
	assert.NotContains(t, output.String(), "Deployment Helper host")
}

func TestDeploymentPreflightRejectsInvalidInputs(t *testing.T) {
	for _, scenario := range []string{"missing archive", "archive directory", "invalid layout", "invalid shared paths", "invalid action"} {
		t.Run(scenario, func(t *testing.T) {
			s := preflightBackend(t)
			options := PreflightOptions{Action: PreflightRollout, Deployment: Deployment{Reference: "release"}}
			switch scenario {
			case "missing archive":
				options.Deployment.Reference = "missing"
			case "archive directory":
				options.Deployment.Reference = t.TempDir()
			case "invalid layout":
				s.directory = "relative/live"
			case "invalid shared paths":
				s.env.SSH.Shared = &shop.EnvironmentSSHSharedConfig{Files: new([]string{"../outside"})}
			case "invalid action":
				options.Action = "unknown"
			}
			var output bytes.Buffer
			require.Error(t, WritePreflight(t.Context(), s, options, &output))
			assert.Empty(t, output.String())
		})
	}
}

func TestDeploymentPreflightRequiresEveryHost(t *testing.T) {
	s := preflightBackend(t)
	backend, err := New(s.root, s.configPath, s.config, &shop.EnvironmentConfig{
		Type: "ssh",
		SSH: &shop.EnvironmentSSHConfig{
			MigrationHost: "a",
			Hosts: map[string]*shop.EnvironmentSSHHostConfig{
				"a": {Host: "first", Directory: "/srv/shop/live"},
				"b": {Host: "second", Directory: "relative/live"},
			},
		},
	})
	require.NoError(t, err)
	var output bytes.Buffer
	require.Error(t, WritePreflight(t.Context(), backend, PreflightOptions{
		Action: PreflightRollout, Deployment: Deployment{Reference: "release"},
	}, &output))
	assert.Empty(t, output.String(), "do not show a partial set of targets")
}

type preflightErrorWriter struct{ err error }

func (w preflightErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestDeploymentPreflightCancellationAndOutputFailure(t *testing.T) {
	s := preflightBackend(t)
	options := PreflightOptions{Action: PreflightRollout, Deployment: Deployment{Reference: "release"}}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var output bytes.Buffer
	require.ErrorIs(t, WritePreflight(ctx, s, options, &output), context.Canceled)
	assert.Empty(t, output.String())
	writeErr := errors.New("closed output")
	require.ErrorIs(t, WritePreflight(t.Context(), s, options, preflightErrorWriter{writeErr}), writeErr)
	assert.NoDirExists(t, filepath.Dir(s.directory))
}

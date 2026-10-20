package deployment

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSSHDeploymentLayout(t *testing.T) {
	for _, directory := range []string{"/srv/shop/current", "/srv/shop/live", "/var/www/shop", "/srv/shop/live/", "/srv/shop/live's shop"} {
		t.Run(directory, func(t *testing.T) {
			s := &SSH{directory: directory}
			root, err := s.deploymentRoot()
			require.NoError(t, err)
			assert.Equal(t, path.Dir(path.Clean(directory)), root)
			assert.Equal(t, path.Base(path.Clean(directory)), s.activeLinkName())
		})
	}
	for _, directory := range []string{"", ".", "relative/live", "/", "/shop", "/srv/shop/releases", "/srv/shop/shared", "/srv/shop/runtime", "/srv/shop/.shopware-cli", "/srv/shop/.current-event", "/srv/shop/live\x00", "/srv/shop/live\n", "/srv/shop\\live"} {
		t.Run(directory, func(t *testing.T) {
			_, err := (&SSH{directory: directory}).deploymentRoot()
			require.Error(t, err)
		})
	}
}

func TestSSHRemoteLayoutRejectsUnsafeLinkNames(t *testing.T) {
	php := deploymentPHP(t)
	root := t.TempDir()
	for _, name := range []string{"", ".", "..", "../outside", "/outside", "releases", "shared", "runtime", ".shopware-cli", ".current-event", "live\x00", "live\n", "live\\path"} {
		t.Run(name, func(t *testing.T) {
			input, err := json.Marshal(map[string]string{"current": name})
			require.NoError(t, err)
			script := strings.TrimPrefix(sshLayoutScript, "<?php\n") + `
try { echo deploymentCurrentPath(json_decode($argv[1], true), $argv[2]); }
catch (Throwable $error) { exit(1); }
`
			_, err = exec.CommandContext(t.Context(), php, "-r", script, "--", string(input), root).Output()
			require.Error(t, err)
		})
	}
}

func TestSSHDeploymentLifecycleWithCustomLink(t *testing.T) {
	for _, name := range []string{"current", "shop", "live's shop"} {
		t.Run(name, func(t *testing.T) {
			s := localDeploymentSSHWithLink(t, name)
			root := filepath.Dir(s.directory)
			rows, err := s.ListRollouts(t.Context())
			require.NoError(t, err)
			assert.Empty(t, rows)
			state, err := s.inspectDeploymentInitialization(t.Context())
			require.NoError(t, err)
			assert.False(t, state.HasCurrent)
			require.NoError(t, s.applyDeploymentInitialization(t.Context(), sshDeploymentInitConfig{
				RuntimeValues: map[string]string{"APP_URL": "https://shop.example.com"},
			}))

			archive := deploymentTestArchive(t, `echo "helper output\n";`)
			for _, release := range []string{"first", "second"} {
				_, err := s.rolloutArchive(t.Context(), Deployment{Reference: release}, archive, io.Discard)
				require.NoError(t, err)
				assert.Equal(t, "releases/"+release, currentRelease(t, s))
			}
			state, err = s.inspectDeploymentInitialization(t.Context())
			require.NoError(t, err)
			assert.True(t, state.HasCurrent)

			failed := deploymentTestArchive(t, "exit(42);")
			_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "failed"}, failed, io.Discard)
			require.Error(t, err)
			assert.Equal(t, "releases/second", currentRelease(t, s))
			candidates, err := s.RollbackCandidates(t.Context())
			require.NoError(t, err)
			require.Len(t, candidates, 2)
			var active []string
			for _, candidate := range candidates {
				if candidate.Active {
					active = append(active, candidate.Deployment.Reference)
				}
			}
			assert.Equal(t, []string{"second"}, active)

			require.NoError(t, os.Remove(archive))
			_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, io.Discard)
			require.NoError(t, err)
			assert.Equal(t, "releases/first", currentRelease(t, s))
			unchanged, err := s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, io.Discard)
			require.NoError(t, err)
			assert.True(t, unchanged.Unchanged)
			rows, err = s.ListRollouts(t.Context())
			require.NoError(t, err)
			require.Len(t, rows, 3)
			assert.True(t, rows[0].Active)
			assert.Equal(t, "first", rows[0].Deployment.Reference)
			assert.False(t, rows[1].Active)
			assert.False(t, rows[2].Active)
			var logs bytes.Buffer
			require.NoError(t, s.WriteDeploymentLogs(t.Context(), Deployment{Reference: "first"}, &logs))
			assert.Equal(t, "helper output\n", logs.String())
			for _, dryRun := range []bool{true, false} {
				pruned, err := s.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0, DryRun: dryRun})
				require.NoError(t, err)
				assert.Equal(t, []Deployment{{Reference: "second"}}, pruned.Deployments)
				assert.Equal(t, "releases/first", currentRelease(t, s))
				assert.DirExists(t, filepath.Join(root, "releases/first"))
				assert.DirExists(t, filepath.Join(root, "releases/failed"))
				if dryRun {
					assert.DirExists(t, filepath.Join(root, "releases/second"))
				}
			}
			assert.NoDirExists(t, filepath.Join(root, "releases/second"))
			if name != "current" {
				_, err := os.Lstat(filepath.Join(root, "current"))
				assert.True(t, os.IsNotExist(err))
			}
		})
	}
}

func TestSSHCustomLinkDoesNotReplaceExistingProject(t *testing.T) {
	s := localDeploymentSSHWithLink(t, "shop")
	testhelper.WriteFile(t, filepath.Join(s.directory, "keep"), "existing application")
	archive := deploymentTestArchive(t, "")
	_, err := s.RolloutDeployment(t.Context(), Deployment{Reference: archive}, io.Discard)
	require.Error(t, err)
	data, err := os.ReadFile(filepath.Join(s.directory, "keep"))
	require.NoError(t, err)
	assert.Equal(t, "existing application", string(data))
}

func TestSSHGroupUsesEachConfiguredActiveLink(t *testing.T) {
	g := localDeploymentGroup(t)
	for i, name := range []string{"live", "shop"} {
		s := g.hosts[i].backend
		s.env.SSH.Directory = filepath.Join(filepath.Dir(s.directory), name)
		backend, err := New(s.root, s.configPath, s.config, s.env)
		require.NoError(t, err)
		g.hosts[i].backend = backend.(*SSH)
	}
	for _, name := range []string{"first", "second"} {
		_, err := g.RolloutDeployment(t.Context(), groupArchive(t, g, name, ""), io.Discard)
		require.NoError(t, err)
	}
	_, err := g.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, io.Discard)
	require.NoError(t, err)
	for _, host := range g.hosts {
		assert.Equal(t, "releases/first", currentRelease(t, host.backend))
		state, err := host.backend.inspectDeploymentInitialization(t.Context())
		require.NoError(t, err)
		assert.True(t, state.HasCurrent)
	}
	rows, err := g.ListRollouts(t.Context())
	require.NoError(t, err)
	var activeHosts []string
	for _, row := range rows {
		if row.Active {
			assert.Equal(t, "first", row.Deployment.Reference)
			activeHosts = append(activeHosts, row.Host)
		}
	}
	assert.ElementsMatch(t, []string{"a", "b"}, activeHosts)
	_, err = g.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0})
	require.NoError(t, err)
	for _, host := range g.hosts {
		assert.DirExists(t, filepath.Join(filepath.Dir(host.backend.directory), "releases/first"))
		assert.NoDirExists(t, filepath.Join(filepath.Dir(host.backend.directory), "releases/second"))
	}
}

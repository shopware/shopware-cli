package deployment

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

// Exercise the real SSH transport and payload without PHP or a remote target.
func fakePruneGroup(t *testing.T, parallelism int) (*sshGroup, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake SSH requires a POSIX shell")
	}
	bin := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(bin, "ssh"), `#!/bin/sh
for arg do command="$arg"; done
exec /bin/sh -c "$command"
`)
	require.NoError(t, os.Chmod(filepath.Join(bin, "ssh"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PRUNE_TEST_ROOT", bin)
	t.Setenv("PRUNE_TEST_SERIAL", "")
	var hosts []sshHost
	for _, name := range []string{"z-empty", "m-failed", "a-success"} {
		worker := filepath.Join(bin, name)
		testhelper.WriteFile(t, worker, `#!/bin/sh
name=${0##*/}
for arg do payload="$arg"; done
printf '%s' "$payload" > "$PRUNE_TEST_ROOT/$name.request"
if [ -n "$PRUNE_TEST_SERIAL" ]; then
    mkdir "$PRUNE_TEST_ROOT/running" || exit 91
    trap 'rmdir "$PRUNE_TEST_ROOT/running"' EXIT
    sleep 0.05
fi
case "$name" in
    m-failed) echo "injected cleanup failure" >&2; exit 42 ;;
    z-empty) echo '{"deployments":[],"artifacts":[]}' ;;
    *) echo '{"deployments":[{"reference":"old"}],"artifacts":["old.tar.gz"]}' ;;
esac
`)
		require.NoError(t, os.Chmod(worker, 0o755))
		backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
			Type: "ssh",
			SSH: &shop.EnvironmentSSHConfig{
				Host: name + ".invalid", Directory: filepath.Join(bin, name+"-root", "current"), PHPBinary: worker,
			},
		})
		require.NoError(t, err)
		hosts = append(hosts, sshHost{name: name, backend: backend.(*SSH)})
	}
	group, err := newSSHGroup(hosts, "m-failed", parallelism)
	require.NoError(t, err)
	return group, bin
}

func TestSSHGroupPrunePartialFailureAndOptions(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "remove", true: "dry run"}[dryRun], func(t *testing.T) {
			g, requests := fakePruneGroup(t, 2)
			options := DeploymentPruneOptions{Keep: 7, DryRun: dryRun}
			result, err := g.PruneDeployments(t.Context(), options)
			require.ErrorContains(t, err, "host m-failed")
			assert.ErrorContains(t, err, "cleanup may be partial or unknown")
			assert.ErrorContains(t, err, "injected cleanup failure")
			assert.Equal(t, []DeploymentPruneHostResult{
				{Host: "a-success", Deployments: []Deployment{{Reference: "old"}}, Artifacts: []string{"old.tar.gz"}},
				{Host: "z-empty", Deployments: []Deployment{}, Artifacts: []string{}},
			}, result.Hosts)
			assert.Nil(t, result.Deployments)
			assert.Nil(t, result.Artifacts)
			for _, host := range g.hosts {
				data, err := os.ReadFile(filepath.Join(requests, host.name+".request"))
				require.NoError(t, err)
				var payload struct {
					Root string `json:"root"`
					DeploymentPruneOptions
				}
				require.NoError(t, json.Unmarshal(data, &payload))
				assert.Equal(t, options, payload.DeploymentPruneOptions)
				assert.Equal(t, filepath.Dir(host.backend.directory), payload.Root)
			}
		})
	}
}

func TestSSHGroupPruneParallelismBound(t *testing.T) {
	g, _ := fakePruneGroup(t, 1)
	t.Setenv("PRUNE_TEST_SERIAL", "1")
	result, err := g.PruneDeployments(t.Context(), DeploymentPruneOptions{})
	require.ErrorContains(t, err, "host m-failed")
	assert.NotContains(t, err.Error(), "host a-success")
	assert.NotContains(t, err.Error(), "host z-empty")
	assert.Len(t, result.Hosts, 2)
}

func TestSSHGroupPruneValidatesBeforeCallingHosts(t *testing.T) {
	g, requests := fakePruneGroup(t, 2)
	result, err := g.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: -1})
	require.ErrorContains(t, err, "must not be negative")
	assert.Empty(t, result.Hosts)
	for _, host := range g.hosts {
		assert.NoFileExists(t, filepath.Join(requests, host.name+".request"))
	}
}

func TestSSHGroupPruneCancelled(t *testing.T) {
	g, requests := fakePruneGroup(t, 1)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := g.PruneDeployments(ctx, DeploymentPruneOptions{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, result.Hosts)
	for _, host := range g.hosts {
		assert.ErrorContains(t, err, "host "+host.name)
		assert.NoFileExists(t, filepath.Join(requests, host.name+".request"))
	}
}

func TestSSHGroupPruneHostLocalRetentionAndSharedTargets(t *testing.T) {
	g := localDeploymentGroup(t)
	shared := filepath.Join(t.TempDir(), "shared")
	testhelper.WriteFile(t, filepath.Join(shared, "persistent"), "shared data")
	before := make(map[string]map[string]string)
	for i, host := range g.hosts {
		e := host.backend
		root := filepath.Dir(e.directory)
		for minute, name := range []string{"old", "middle", "new"} {
			addPruneRelease(t, e, name, name+"-event", pruneChecksum(name), "successful", minute)
			require.NoError(t, os.Symlink(shared, filepath.Join(root, "releases", name, "shared-data")))
		}
		active := []string{"old", "middle"}[i]
		require.NoError(t, os.Symlink("releases/"+active, e.directory))
		before[host.name] = pruneSnapshot(t, root)
	}
	sharedBefore := pruneSnapshot(t, shared)
	preview, err := g.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 1, DryRun: true})
	require.NoError(t, err)
	require.Len(t, preview.Hosts, 2)
	assert.Equal(t, []Deployment{{Reference: "middle"}}, preview.Hosts[0].Deployments)
	assert.Equal(t, []Deployment{{Reference: "old"}}, preview.Hosts[1].Deployments)
	for _, host := range g.hosts {
		assert.Equal(t, before[host.name], pruneSnapshot(t, filepath.Dir(host.backend.directory)))
	}
	result, err := g.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 1})
	require.NoError(t, err)
	assert.Equal(t, preview, result)
	for i, host := range g.hosts {
		root := filepath.Dir(host.backend.directory)
		active := []string{"old", "middle"}[i]
		assert.Equal(t, "releases/"+active, currentRelease(t, host.backend))
		assert.DirExists(t, filepath.Join(root, "releases", active))
		assert.DirExists(t, filepath.Join(root, "releases", "new"))
		assert.NoDirExists(t, filepath.Join(root, "releases", result.Hosts[i].Deployments[0].Reference))
	}
	assert.Equal(t, sharedBefore, pruneSnapshot(t, shared))
	result, err = g.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 1})
	require.NoError(t, err)
	require.Len(t, result.Hosts, 2)
	for _, host := range result.Hosts {
		assert.Empty(t, host.Deployments)
		assert.Empty(t, host.Artifacts)
	}
}

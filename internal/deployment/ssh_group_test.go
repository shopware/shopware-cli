package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func localDeploymentGroup(t *testing.T) *sshGroup {
	t.Helper()
	a, b := localDeploymentSSH(t), localDeploymentSSH(t)
	for _, s := range []*SSH{a, b} {
		_, directories, err := s.env.SSH.SharedPaths()
		require.NoError(t, err)
		for _, directory := range directories {
			require.NoError(t, os.MkdirAll(filepath.Join(filepath.Dir(s.directory), "shared", directory), 0o755))
		}
	}
	g, err := newSSHGroup([]sshHost{{name: "b", backend: b}, {name: "a", backend: a}}, "a", 2)
	require.NoError(t, err)
	t.Setenv("COHORT_HELPER_COUNTER", filepath.Join(t.TempDir(), "helper-counter"))
	t.Setenv("COHORT_CHECK_LOCK", "")
	t.Setenv("COHORT_BREAK_ACTIVATION", "")
	t.Setenv("COHORT_EXPECT_SHARED", "")
	return g
}

func groupArchive(t *testing.T, g *sshGroup, name, helper string) Deployment {
	t.Helper()
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), "{}")
	testhelper.WriteFile(t, filepath.Join(root, "vendor/bin/shopware-deployment-helper"), `<?php
file_put_contents(getenv('COHORT_HELPER_COUNTER'), "helper\n", FILE_APPEND | LOCK_EX);
echo "global helper\n";
`+helper)
	testhelper.WriteFile(t, filepath.Join(root, "bin/console"), `<?php
if (array_slice($argv, 1) !== ['cache:warmup', '--no-interaction']) { exit(90); }
if ($path = getenv('COHORT_CHECK_LOCK')) {
    $lock = fopen($path, 'r');
    if (flock($lock, LOCK_EX | LOCK_NB)) { exit(91); }
    fclose($lock);
}
if (getenv('COHORT_BREAK_ACTIVATION')) { mkdir(dirname(getcwd(), 2) . '/current'); }
if (($expected = getenv('COHORT_EXPECT_SHARED')) && file_get_contents('files/shared-marker') !== $expected) { exit(92); }
file_put_contents('warmed', 'yes');
echo "local warmup\n";
`)
	filename := filepath.Join(g.hosts[g.migration].backend.root, ".shopware-cli/deployments", name+".tar.gz")
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	out, err := os.Create(filename)
	require.NoError(t, err)
	require.NoError(t, archiver.WriteTarGz(t.Context(), out, root, nil))
	require.NoError(t, out.Close())
	return Deployment{Reference: name}
}

func groupCounter(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(os.Getenv("COHORT_HELPER_COUNTER"))
	require.NoError(t, err)
	return string(data)
}

func assertGroupLocksReleased(t *testing.T, g *sshGroup) {
	t.Helper()
	for _, host := range g.hosts {
		file := filepath.Join(filepath.Dir(host.backend.directory), ".shopware-cli/deployment.lock")
		out, err := exec.CommandContext(t.Context(), deploymentPHP(t), "-r",
			`$f = fopen($argv[1], 'c'); exit(flock($f, LOCK_EX | LOCK_NB) ? 0 : 42);`, "--", file).CombinedOutput()
		require.NoError(t, err, "%s: %s", host.name, out)
	}
}

func TestSSHGroupRolloutAndRollback(t *testing.T) {
	g := localDeploymentGroup(t)
	signals, _, _ := phpRestartTestCommands(t)
	first := groupArchive(t, g, "first", "")
	var logs bytes.Buffer
	rollout, err := g.RolloutDeployment(t.Context(), first, &logs)
	require.NoError(t, err, logs.String())
	assert.True(t, rollout.Active)
	assert.False(t, rollout.Unchanged)
	assert.Equal(t, "helper\n", groupCounter(t))
	assert.Contains(t, logs.String(), "[a] global helper")
	assert.Contains(t, logs.String(), "[b] local warmup")
	for _, host := range g.hosts {
		assert.Equal(t, "releases/first", currentRelease(t, host.backend))
		rows, err := host.backend.ListRollouts(t.Context())
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, rollout.Reference, rows[0].Reference)
	}
	resetCalls, err := os.ReadFile(signals)
	require.NoError(t, err)
	assert.Equal(t, "-TERM\n101\n-TERM\n202\n-TERM\n101\n-TERM\n202\n", string(resetCalls))

	unchanged, err := g.RolloutDeployment(t.Context(), first, nil)
	require.NoError(t, err)
	assert.True(t, unchanged.Unchanged)
	assert.Equal(t, rollout.Reference, unchanged.Reference)
	afterNoop, err := os.ReadFile(signals)
	require.NoError(t, err)
	assert.Equal(t, resetCalls, afterNoop)
	assert.Equal(t, "helper\n", groupCounter(t))

	before, err := g.RollbackCandidates(t.Context())
	require.NoError(t, err)
	require.Len(t, before, 1)
	second := groupArchive(t, g, "second", "")
	_, err = g.RolloutDeployment(t.Context(), second, nil)
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("helper\n", 2), groupCounter(t))
	for _, host := range g.hosts {
		require.NoError(t, os.RemoveAll(filepath.Join(filepath.Dir(host.backend.directory), ".shopware-cli/artifacts")))
		// Rollback must not execute either helper or follower console.
		for _, file := range []string{"bin/console", "vendor/bin/shopware-deployment-helper"} {
			require.NoError(t, os.Remove(filepath.Join(filepath.Dir(host.backend.directory), "releases/first", file)))
		}
	}
	require.NoError(t, os.RemoveAll(filepath.Join(g.hosts[g.migration].backend.root, ".shopware-cli/deployments")))
	rollback, err := g.ActivateDeployment(t.Context(), first, nil)
	require.NoError(t, err)
	assert.True(t, rollback.Active)
	assert.NotEqual(t, rollout.Reference, rollback.Reference)
	assert.Equal(t, strings.Repeat("helper\n", 2), groupCounter(t))
	for _, host := range g.hosts {
		assert.Equal(t, "releases/first", currentRelease(t, host.backend))
	}
	candidates, err := g.RollbackCandidates(t.Context())
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, "second", candidates[0].Deployment.Reference)
	assert.Equal(t, before[0].CreatedAt, candidates[1].CreatedAt)
	assert.True(t, candidates[1].Active)
	noopRollback, err := g.ActivateDeployment(t.Context(), first, nil)
	require.NoError(t, err)
	assert.True(t, noopRollback.Unchanged)
	assert.Equal(t, rollback.Reference, noopRollback.Reference)
	history, err := g.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, history, 6)
	assert.NotEmpty(t, history[0].Host)
	for i := 1; i < len(history); i++ {
		assert.False(t, history[i].DeployedAt.After(*history[i-1].DeployedAt))
	}
	assertGroupLocksReleased(t, g)
}

func TestSSHGroupSharesHelperOutputBeforeFollowerWarmup(t *testing.T) {
	g := localDeploymentGroup(t)
	shared := filepath.Join(t.TempDir(), "shared-marker")
	require.NoError(t, os.WriteFile(shared, []byte("unprepared"), 0o600))
	for _, host := range g.hosts {
		// Hard links model a shared file without requiring privileged mounts.
		require.NoError(t, os.Link(shared, filepath.Join(filepath.Dir(host.backend.directory), "shared/files/shared-marker")))
	}
	t.Setenv("COHORT_EXPECT_SHARED", "prepared")
	artifact := groupArchive(t, g, "shared-output", `file_put_contents('files/shared-marker', 'prepared');`)

	var output bytes.Buffer
	_, err := g.RolloutDeployment(t.Context(), artifact, &output)
	require.NoError(t, err, output.String())
	assert.Equal(t, "helper\n", groupCounter(t))
	for _, host := range g.hosts {
		data, err := os.ReadFile(filepath.Join(host.backend.directory, "files/shared-marker"))
		require.NoError(t, err)
		assert.Equal(t, "prepared", string(data))
	}
	assert.FileExists(t, filepath.Join(g.hosts[1].backend.directory, "warmed"))
	assertGroupLocksReleased(t, g)
}

func TestSSHGroupAbortsBeforeActivation(t *testing.T) {
	for _, failure := range []string{"staging", "helper", "warmup"} {
		t.Run(failure, func(t *testing.T) {
			g := localDeploymentGroup(t)
			helper := ""
			if failure == "helper" {
				helper = "exit(42);"
			}
			deployment := groupArchive(t, g, "first", helper)
			switch failure {
			case "staging":
				_, dirs, err := g.hosts[1].backend.env.SSH.SharedPaths()
				require.NoError(t, err)
				require.NoError(t, os.RemoveAll(filepath.Join(filepath.Dir(g.hosts[1].backend.directory), "shared", dirs[0])))
			case "warmup":
				// A freely available lock must fail the follower's held-lock check.
				file := filepath.Join(t.TempDir(), "not-held")
				require.NoError(t, os.WriteFile(file, nil, 0o600))
				t.Setenv("COHORT_CHECK_LOCK", file)
			}
			var output bytes.Buffer
			result, err := g.RolloutDeployment(t.Context(), deployment, &output)
			require.Error(t, err, output.String())
			assert.False(t, result.Active)
			assert.Contains(t, err.Error(), "not activated")
			for _, host := range g.hosts {
				_, err := os.Lstat(host.backend.directory)
				assert.True(t, os.IsNotExist(err))
			}
			if failure == "staging" {
				assert.NoFileExists(t, os.Getenv("COHORT_HELPER_COUNTER"))
			} else {
				assert.Equal(t, "helper\n", groupCounter(t))
			}
			assertGroupLocksReleased(t, g)
		})
	}
}

func TestSSHGroupPartialActivation(t *testing.T) {
	g := localDeploymentGroup(t)
	deployment := groupArchive(t, g, "first", "")
	t.Setenv("COHORT_BREAK_ACTIVATION", "1")
	result, err := g.RolloutDeployment(t.Context(), deployment, nil)
	require.ErrorContains(t, err, "a=activated, b=failed activation; state unknown")
	assert.False(t, result.Active)
	assert.Equal(t, "releases/first", currentRelease(t, g.hosts[0].backend))
	history, err := g.hosts[0].backend.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.True(t, history[0].Active, "abort cleanup must not relabel a committed activation")
	assert.DirExists(t, g.hosts[1].backend.directory)
	assertGroupLocksReleased(t, g)
}

func TestSSHGroupHoldsUnchangedMemberLock(t *testing.T) {
	g := localDeploymentGroup(t)
	deployment := groupArchive(t, g, "first", "")
	leader := g.hosts[g.migration].backend
	_, err := leader.RolloutDeployment(t.Context(), deployment, nil)
	require.NoError(t, err)
	t.Setenv("COHORT_CHECK_LOCK", filepath.Join(filepath.Dir(leader.directory), ".shopware-cli/deployment.lock"))
	result, err := g.RolloutDeployment(t.Context(), deployment, nil)
	require.NoError(t, err)
	assert.True(t, result.Active)
	assert.False(t, result.Unchanged)
	assert.Equal(t, "helper\n", groupCounter(t), "already prepared migration member skips helper")
	assertGroupLocksReleased(t, g)
}

func TestSSHGroupRollbackRejectsChecksumDrift(t *testing.T) {
	g := localDeploymentGroup(t)
	deployment := groupArchive(t, g, "first", "")
	_, err := g.RolloutDeployment(t.Context(), deployment, nil)
	require.NoError(t, err)
	candidates, err := g.rollbackCandidates(t.Context())
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	second := groupArchive(t, g, "second", "")
	_, err = g.RolloutDeployment(t.Context(), second, nil)
	require.NoError(t, err)
	root := filepath.Dir(g.hosts[1].backend.directory)
	stateFile := filepath.Join(root, ".shopware-cli/releases/first.json")
	var state map[string]any
	content, err := os.ReadFile(stateFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(content, &state))
	eventFile := filepath.Join(root, ".shopware-cli/rollouts", state["reference"].(string)+".json")
	for _, filename := range []string{stateFile, eventFile} {
		content, err := os.ReadFile(filename)
		require.NoError(t, err)
		var metadata map[string]any
		require.NoError(t, json.Unmarshal(content, &metadata))
		metadata["sha256"] = strings.Repeat("0", 64)
		content, err = json.Marshal(metadata)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filename, content, 0o600))
	}
	eligible, err := g.RollbackCandidates(t.Context())
	require.NoError(t, err)
	require.Len(t, eligible, 1)
	assert.Equal(t, "second", eligible[0].Deployment.Reference)
	_, err = g.ActivateDeployment(t.Context(), deployment, nil)
	require.ErrorContains(t, err, "same checksum")
	// Even a candidate discovered before the drift must be revalidated under
	// every held lock before a single current link may change.
	_, err = g.coordinate(t.Context(), deployment, sshRolloutInput{
		Action: "activate", Release: "first", SHA256: candidates[0].SHA256,
	}, nil, nil)
	require.Error(t, err)
	for _, host := range g.hosts {
		assert.Equal(t, "releases/second", currentRelease(t, host.backend))
	}
	assertGroupLocksReleased(t, g)
}

func TestSSHGroupHistoryReturnsAvailableHosts(t *testing.T) {
	g := localDeploymentGroup(t)
	deployment := groupArchive(t, g, "first", "")
	_, err := g.RolloutDeployment(t.Context(), deployment, nil)
	require.NoError(t, err)
	g.hosts[1].backend.directory = "/invalid-layout"
	rows, err := g.ListRollouts(t.Context())
	require.ErrorContains(t, err, "host b: unavailable; deployment state unknown")
	require.Len(t, rows, 1)
	assert.Equal(t, "a", rows[0].Host)
}

func TestSSHGroupParallelism(t *testing.T) {
	for _, limit := range []int{0, 1, 2} {
		g, err := newSSHGroup([]sshHost{{"a", &SSH{}}, {"b", &SSH{}}, {"c", &SSH{}}}, "a", limit)
		require.NoError(t, err)
		var running, maximum atomic.Int32
		require.NoError(t, g.parallel(t.Context(), func(int) error {
			current := running.Add(1)
			for old := maximum.Load(); old < current && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
			}
			time.Sleep(10 * time.Millisecond)
			running.Add(-1)
			return nil
		}))
		expected := limit
		if expected == 0 {
			expected = 2
		}
		assert.LessOrEqual(t, maximum.Load(), int32(expected))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, g.parallel(ctx, func(int) error { return ctx.Err() }), context.Canceled)
	}
}

func TestSSHCoordinatedDisconnectNeverActivates(t *testing.T) {
	for _, phase := range []string{"staged", "ready", "cancelled"} {
		t.Run(phase, func(t *testing.T) {
			g := localDeploymentGroup(t)
			deployment := groupArchive(t, g, "first", "")
			leader := g.hosts[g.migration].backend
			input, data := cachetoolTestInput(t, leader,
				resolveDeploymentArchive(leader.root, deployment.Reference), "first", "cohort-disconnect", nil)
			input.Coordinated = true
			session, err := leader.openDeploymentSession(t.Context(), input, io.Discard)
			require.NoError(t, err)
			require.NoError(t, session.stage(t.Context(), bytes.NewReader(data)))
			if phase != "staged" {
				require.NoError(t, session.prepare(t.Context()))
			}
			if phase == "cancelled" {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				require.ErrorIs(t, session.activate(ctx), context.Canceled)
			}
			require.Error(t, session.close(), "EOF must abort unapproved preparation/activation")
			_, err = os.Lstat(leader.directory)
			assert.True(t, os.IsNotExist(err))
			file := filepath.Join(filepath.Dir(leader.directory), ".shopware-cli/deployment.lock")
			output, err := exec.CommandContext(t.Context(), deploymentPHP(t), "-r",
				`$f = fopen($argv[1], 'r'); exit(flock($f, LOCK_EX | LOCK_NB) ? 0 : 42);`, "--", file).CombinedOutput()
			require.NoError(t, err, string(output))
			if phase == "staged" {
				assert.NoFileExists(t, os.Getenv("COHORT_HELPER_COUNTER"))
			}
		})
	}
}

func TestSSHGroupUsesDesignatedMigrationHost(t *testing.T) {
	g := localDeploymentGroup(t)
	g.migration = 1
	deployment := groupArchive(t, g, "first", "")
	var logs bytes.Buffer
	_, err := g.RolloutDeployment(t.Context(), deployment, &logs)
	require.NoError(t, err, logs.String())
	assert.Contains(t, logs.String(), "[b] global helper")
	assert.Contains(t, logs.String(), "[a] local warmup")
	assert.Equal(t, "helper\n", groupCounter(t))
	var backend Backend = g
	_, canInitialize := backend.(Initializer)
	_, canPrune := backend.(DeploymentPruner)
	assert.False(t, canInitialize)
	assert.False(t, canPrune)
}

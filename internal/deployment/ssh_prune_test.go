package deployment

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func pruneChecksum(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}

func writePruneJSON(t *testing.T, file string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	testhelper.WriteFile(t, file, string(data))
}

func addPruneRelease(t *testing.T, e *SSH, name, reference, checksum, status string, minute int) {
	t.Helper()
	root := filepath.Dir(e.directory)
	stamp := time.Date(2026, 9, 22, 10, minute, 0, 0, time.UTC).Format(time.RFC3339Nano)
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/deployment.lock"), "")
	testhelper.WriteFile(t, filepath.Join(root, "releases", name, "app.txt"), name)
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/logs", name+".log"), "logs for "+name)
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/artifacts", checksum+".tar.gz"), "cached archive")
	metadata := map[string]any{
		"reference": reference, "release": name, "deployment": name, "sha256": checksum,
		"status": status, "created_at": stamp,
	}
	state := map[string]any{"release": name, "sha256": checksum, "ready": false}
	if status == "successful" {
		metadata["deployed_at"] = stamp
		state["reference"] = reference
		state["ready"] = true
		state["deployed_at"] = stamp
	}
	writePruneJSON(t, filepath.Join(root, ".shopware-cli/rollouts", reference+".json"), metadata)
	writePruneJSON(t, filepath.Join(root, ".shopware-cli/releases", name+".json"), state)
}

func pruneSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	require.NoError(t, filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := info.Mode().String() + " " + info.ModTime().String()
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += " " + target
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += " " + string(data)
		}
		result[path] = value
		return nil
	}))
	return result
}

func TestSSHPruneRetentionAndDryRun(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	addPruneRelease(t, e, "active-old", "active-event", pruneChecksum("active"), "successful", 1)
	addPruneRelease(t, e, "old", "old-event", pruneChecksum("old"), "successful", 2)
	addPruneRelease(t, e, "shared-old", "shared-old-event", pruneChecksum("shared"), "successful", 3)
	addPruneRelease(t, e, "recent", "recent-event", pruneChecksum("shared"), "successful", 4)
	addPruneRelease(t, e, "newest", "newest-first", pruneChecksum("newest"), "successful", 5)
	addPruneRelease(t, e, "newest", "newest-again", pruneChecksum("newest"), "successful", 6)
	addPruneRelease(t, e, "failed", "failed-event", pruneChecksum("failed"), "failed", 7)
	addPruneRelease(t, e, "incomplete", "preparing-event", pruneChecksum("incomplete"), "preparing", 8)
	require.NoError(t, os.Symlink("releases/active-old", e.directory))
	require.NoError(t, os.Symlink("../../shared", filepath.Join(root, "releases/old/shared-data")))
	localArchive := filepath.Join(t.TempDir(), "old.tar.gz")
	testhelper.WriteFile(t, localArchive, "local archive must remain")
	orphan := pruneChecksum("orphan") + ".tar.gz"
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/artifacts", orphan), "unused archive")
	testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/artifacts/upload.tmp"), "incomplete upload")

	before := pruneSnapshot(t, root)
	preview, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 2, DryRun: true})
	require.NoError(t, err)
	assert.ElementsMatch(t, []Deployment{{Reference: "old"}, {Reference: "shared-old"}}, preview.Deployments)
	assert.ElementsMatch(t, []string{pruneChecksum("old") + ".tar.gz", orphan}, preview.Artifacts)
	assert.Equal(t, before, pruneSnapshot(t, root), "dry-run must leave the entire target unchanged")

	result, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 2})
	require.NoError(t, err)
	assert.Equal(t, preview, result)
	for _, name := range []string{"old", "shared-old"} {
		assert.NoDirExists(t, filepath.Join(root, "releases", name))
		assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/releases", name+".json"))
		assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/logs", name+".log"))
		assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/rollouts", name+"-event.json"))
	}
	for _, name := range []string{"active-old", "recent", "newest", "failed", "incomplete"} {
		assert.DirExists(t, filepath.Join(root, "releases", name))
		assert.FileExists(t, filepath.Join(root, ".shopware-cli/logs", name+".log"))
	}
	for _, name := range []string{"active", "shared", "newest", "failed", "incomplete"} {
		assert.FileExists(t, filepath.Join(root, ".shopware-cli/artifacts", pruneChecksum(name)+".tar.gz"))
	}
	assert.FileExists(t, filepath.Join(root, ".shopware-cli/rollouts/newest-first.json"))
	assert.FileExists(t, filepath.Join(root, ".shopware-cli/rollouts/newest-again.json"))
	assert.FileExists(t, filepath.Join(root, ".shopware-cli/artifacts/upload.tmp"))
	assert.Equal(t, "releases/active-old", currentRelease(t, e))
	assert.FileExists(t, filepath.Join(root, "shared/public/media/upload"))
	assert.FileExists(t, localArchive)
	result, err = e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 2})
	require.NoError(t, err)
	assert.Empty(t, result.Deployments)
	assert.Empty(t, result.Artifacts)
}

func TestSSHPruneRealRollouts(t *testing.T) {
	e := localDeploymentSSH(t)
	archive := deploymentTestArchive(t, `echo "helper output\n";`)
	for _, name := range []string{"happy-euclid", "focused-turing", "happy-euclid"} {
		_, err := e.rolloutArchive(t.Context(), Deployment{Reference: name}, archive, io.Discard)
		require.NoError(t, err)
	}
	result, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0})
	require.NoError(t, err)
	assert.Equal(t, []Deployment{{Reference: "focused-turing"}}, result.Deployments)
	assert.Empty(t, result.Artifacts, "active deployment still references the same archive")
	history, err := e.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, history, 2)
	for _, rollout := range history {
		assert.Equal(t, "happy-euclid", rollout.Deployment.Reference)
	}
	assert.True(t, history[0].Active)
	assert.False(t, history[1].Active)
	var logs strings.Builder
	require.NoError(t, e.WriteDeploymentLogs(t.Context(), Deployment{Reference: "happy-euclid"}, &logs))
	assert.Equal(t, "helper output\n", logs.String())
	assert.FileExists(t, archive)
	_, err = e.rolloutArchive(t.Context(), Deployment{Reference: "focused-turing"}, archive, io.Discard)
	require.NoError(t, err, "fully pruned names can be prepared again from a local archive")
	assert.Equal(t, "releases/focused-turing", currentRelease(t, e))
}

func TestSSHPruneProtectsUnmanagedReleases(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	addPruneRelease(t, e, "managed", "event", pruneChecksum("managed"), "successful", 1)
	testhelper.WriteFile(t, filepath.Join(root, "releases/unmanaged/app.txt"), "not owned by the CLI")
	orphan := filepath.Join(root, ".shopware-cli/artifacts", pruneChecksum("unknown")+".tar.gz")
	testhelper.WriteFile(t, orphan, "may belong to unmanaged release")
	result, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0})
	require.NoError(t, err)
	assert.Equal(t, []Deployment{{Reference: "managed"}}, result.Deployments)
	assert.Empty(t, result.Artifacts, "cannot prove that unmanaged releases don't reference cached archives")
	assert.DirExists(t, filepath.Join(root, "releases/unmanaged"))
	assert.FileExists(t, orphan)
}

func TestSSHPruneProtectsPendingAndFailedArtifactReferences(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	addPruneRelease(t, e, "pending", "old-success", pruneChecksum("pending"), "successful", 1)
	statePath := filepath.Join(root, ".shopware-cli/releases/pending.json")
	data, err := os.ReadFile(statePath)
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, json.Unmarshal(data, &state))
	state["activation_pending"] = "interrupted"
	writePruneJSON(t, statePath, state)
	// A failed upload can leave cached bytes and history without a release directory.
	writePruneJSON(t, filepath.Join(root, ".shopware-cli/rollouts/failed-upload.json"), map[string]any{
		"reference": "failed-upload", "release": "never-prepared", "deployment": "never-prepared",
		"sha256": pruneChecksum("failed-upload"), "created_at": "2026-09-22T11:00:00Z", "status": "failed",
	})
	failedArtifact := filepath.Join(root, ".shopware-cli/artifacts", pruneChecksum("failed-upload")+".tar.gz")
	testhelper.WriteFile(t, failedArtifact, "retained failure")
	result, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0})
	require.NoError(t, err)
	assert.Empty(t, result.Deployments)
	assert.Empty(t, result.Artifacts)
	assert.DirExists(t, filepath.Join(root, "releases/pending"))
	assert.FileExists(t, failedArtifact)
}

func TestSSHPruneRejectsUnsafeStorageBeforeDeletion(t *testing.T) {
	for _, scenario := range []string{"corrupt metadata", "symlinked metadata", "symlinked logs", "real current", "outside current"} {
		t.Run(scenario, func(t *testing.T) {
			e := localDeploymentSSH(t)
			root := filepath.Dir(e.directory)
			addPruneRelease(t, e, "old", "event", pruneChecksum("old"), "successful", 1)
			outside := t.TempDir()
			testhelper.WriteFile(t, filepath.Join(outside, "keep"), "outside data")
			switch scenario {
			case "corrupt metadata":
				testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli/rollouts/event.json"), "{")
			case "symlinked metadata":
				require.NoError(t, os.Remove(filepath.Join(root, ".shopware-cli/releases/old.json")))
				require.NoError(t, os.Symlink(filepath.Join(outside, "keep"), filepath.Join(root, ".shopware-cli/releases/old.json")))
			case "symlinked logs":
				require.NoError(t, os.Remove(filepath.Join(root, ".shopware-cli/logs/old.log")))
				require.NoError(t, os.Symlink(filepath.Join(outside, "keep"), filepath.Join(root, ".shopware-cli/logs/old.log")))
			case "real current":
				testhelper.WriteFile(t, filepath.Join(e.directory, "keep"), "active application")
			case "outside current":
				require.NoError(t, os.Symlink(outside, e.directory))
			}
			before := pruneSnapshot(t, root)
			_, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0})
			require.Error(t, err)
			assert.Equal(t, before, pruneSnapshot(t, root), "unsafe plan must not partially delete releases")
			assert.FileExists(t, filepath.Join(outside, "keep"))
		})
	}
}

func TestSSHPruneUsesDeploymentLock(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	addPruneRelease(t, e, "old", "event", pruneChecksum("old"), "successful", 1)
	// A pipe keeps the lock holder alive until cleanup, without sleeps or networking.
	command := exec.CommandContext(t.Context(), deploymentPHP(t), "-r", `
$lock = fopen($argv[1], 'r');
if (!flock($lock, LOCK_EX | LOCK_NB)) { exit(1); }
echo "READY\n"; fflush(STDOUT); fgets(STDIN);
`, filepath.Join(root, ".shopware-cli/deployment.lock"))
	stdin, err := command.StdinPipe()
	require.NoError(t, err)
	stdout, err := command.StdoutPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = command.Wait()
	})
	ready := make([]byte, len("READY\n"))
	_, err = io.ReadFull(stdout, ready)
	require.NoError(t, err)
	assert.Equal(t, "READY\n", string(ready))
	for _, dryRun := range []bool{false, true} {
		_, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0, DryRun: dryRun})
		require.Error(t, err)
		assert.DirExists(t, filepath.Join(root, "releases/old"))
	}
}

func TestSSHPruneValidationAndEmptyTarget(t *testing.T) {
	e := localDeploymentSSH(t)
	before := pruneSnapshot(t, filepath.Dir(e.directory))
	result, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 5})
	require.NoError(t, err)
	assert.Empty(t, result.Deployments)
	assert.Empty(t, result.Artifacts)
	assert.Equal(t, before, pruneSnapshot(t, filepath.Dir(e.directory)))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = e.PruneDeployments(ctx, DeploymentPruneOptions{Keep: 5})
	require.ErrorIs(t, err, context.Canceled)
	t.Setenv("PATH", t.TempDir())
	_, err = e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: -1})
	require.ErrorContains(t, err, "must not be negative")
}

func TestSSHPruneDefaultRetentionCountsDistinctReleases(t *testing.T) {
	e := localDeploymentSSH(t)
	for i := range 7 {
		name := fmt.Sprintf("release-%d", i)
		addPruneRelease(t, e, name, name+"-event", pruneChecksum(name), "successful", i)
	}
	require.NoError(t, os.Symlink("releases/release-0", e.directory))
	result, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 5, DryRun: true})
	require.NoError(t, err)
	assert.Equal(t, []Deployment{{Reference: "release-1"}}, result.Deployments)
}

func TestSSHPruneResumesInterruptedCleanup(t *testing.T) {
	for _, stage := range []string{"before tree removal", "after tree removal", "after history removal", "marker only"} {
		t.Run(stage, func(t *testing.T) {
			e := localDeploymentSSH(t)
			root := filepath.Dir(e.directory)
			hash := pruneChecksum("old")
			addPruneRelease(t, e, "old", "event", hash, "successful", 1)
			writePruneJSON(t, filepath.Join(root, ".shopware-cli/releases/old.json"), map[string]any{
				"release": "old", "sha256": hash, "ready": false, "pruning": true,
			})
			if stage != "before tree removal" {
				require.NoError(t, os.RemoveAll(filepath.Join(root, "releases/old")))
			}
			if stage == "after history removal" || stage == "marker only" {
				require.NoError(t, os.Remove(filepath.Join(root, ".shopware-cli/rollouts/event.json")))
			}
			if stage == "marker only" {
				require.NoError(t, os.Remove(filepath.Join(root, ".shopware-cli/logs/old.log")))
			}
			before := pruneSnapshot(t, root)
			preview, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 5, DryRun: true})
			require.NoError(t, err)
			assert.Equal(t, []Deployment{{Reference: "old"}}, preview.Deployments)
			assert.Equal(t, before, pruneSnapshot(t, root))
			result, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 5})
			require.NoError(t, err)
			assert.Equal(t, preview, result)
			assert.NoDirExists(t, filepath.Join(root, "releases/old"))
			assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/releases/old.json"))
			assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/rollouts/event.json"))
			assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/logs/old.log"))
			assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/artifacts", hash+".tar.gz"))
		})
	}
}

func TestSSHPruneRetriesAfterLogRemovalFailure(t *testing.T) {
	e := localDeploymentSSH(t)
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	root := filepath.Dir(e.directory)
	addPruneRelease(t, e, "old", "event", pruneChecksum("old"), "successful", 1)
	logs := filepath.Join(root, ".shopware-cli/logs")
	require.NoError(t, os.Chmod(logs, 0o500))
	t.Cleanup(func() { _ = os.Chmod(logs, 0o700) })
	_, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0})
	require.Error(t, err)
	assert.NoDirExists(t, filepath.Join(root, "releases/old"))
	assert.FileExists(t, filepath.Join(root, ".shopware-cli/releases/old.json"))
	assert.FileExists(t, filepath.Join(logs, "old.log"))
	require.NoError(t, os.Chmod(logs, 0o700))
	result, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0})
	require.NoError(t, err)
	assert.Equal(t, []Deployment{{Reference: "old"}}, result.Deployments)
	assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/releases/old.json"))
	assert.NoFileExists(t, filepath.Join(logs, "old.log"))
}

func TestSSHPruneNeverResumesActiveReleaseCleanup(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	addPruneRelease(t, e, "active", "event", pruneChecksum("active"), "successful", 1)
	require.NoError(t, os.Symlink("releases/active", e.directory))
	writePruneJSON(t, filepath.Join(root, ".shopware-cli/releases/active.json"), map[string]any{
		"release": "active", "sha256": pruneChecksum("active"), "ready": false, "pruning": true,
	})
	before := pruneSnapshot(t, root)
	_, err := e.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0})
	require.ErrorContains(t, err, "active release")
	assert.Equal(t, before, pruneSnapshot(t, root))
}

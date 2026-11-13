package ssh

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ci"
)

// Drives the packaged commands through a rollout, a rollback and retention
// with local PHP, using the same client protocol as a remote host.
func TestDeploymentPayloadLifecycle(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		// Sealed releases are read-only.
		require.NoError(t, exec.CommandContext(context.WithoutCancel(t.Context()), "chmod", "-R", "u+rwX", root).Run())
	})
	archive := deploymentTestArchive(t, map[string]string{
		"vendor/bin/shopware-deployment-helper": "<?php echo 'helper ran', PHP_EOL;",
		"public/index.php":                      "<?php",
	})
	checksum := sha256.Sum256(archive)
	deploy := func(input sshRolloutInput) Rollout {
		t.Helper()
		input.Root = root
		input.Deployment = input.Release
		cmd, program := localDeploymentCommand(t, "deploy", input)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		stdin, err := cmd.StdinPipe()
		require.NoError(t, err)
		stdout, err := cmd.StdoutPipe()
		require.NoError(t, err)
		require.NoError(t, cmd.Start())
		_, err = io.Copy(stdin, program)
		require.NoError(t, err)
		reader := bufio.NewReader(stdout)
		result, err := exchangeSSHDeployment(t.Context(), stdin, reader, bytes.NewReader(archive), input, ci.New(&stderr))
		if err == nil && input.Action == sshDeploymentActivate {
			err = json.NewDecoder(reader).Decode(&result)
		}
		require.NoError(t, stdin.Close())
		require.NoError(t, cmd.Wait(), "%s", stderr.String())
		require.NoError(t, err, "%s", stderr.String())
		return result
	}
	rollout := func(release, reference string) Rollout {
		t.Helper()
		return deploy(sshRolloutInput{
			Release: release, Reference: reference, SHA256: hex.EncodeToString(checksum[:]), Size: int64(len(archive)),
			SharedFiles: []string{".env.local"}, SharedDirectories: []string{"files"},
		})
	}
	active := func() string {
		t.Helper()
		target, err := os.Readlink(filepath.Join(root, "current"))
		require.NoError(t, err)
		return target
	}

	assert.Equal(t, "event-1", rollout("one", "event-1").Reference)
	assert.Equal(t, "releases/one", active())
	// The second release reuses the cached artifact instead of uploading it again.
	assert.True(t, rollout("two", "event-2").Active)
	assert.Equal(t, "releases/two", active())
	assert.DirExists(t, filepath.Join(root, "shared", "files"))

	var rollouts []Rollout
	require.NoError(t, json.Unmarshal(runLocalDeploymentCommand(t, "list", sshDeploymentListInput{Root: root, Current: "current"}), &rollouts))
	require.Len(t, rollouts, 2)
	assert.Equal(t, "event-2", rollouts[0].Reference)
	assert.True(t, rollouts[0].Active)
	assert.False(t, rollouts[1].Active)

	var candidates []sshCandidate
	output := runLocalDeploymentCommand(t, "deploy", sshRolloutInput{Root: root, Action: sshDeploymentCandidates})
	require.NoError(t, json.Unmarshal(output, &candidates))
	require.Len(t, candidates, 2)
	assert.Equal(t, "two", candidates[0].Deployment.Reference)
	assert.True(t, candidates[0].Active)
	assert.Equal(t, hex.EncodeToString(checksum[:]), candidates[1].SHA256)

	rollback := deploy(sshRolloutInput{Action: sshDeploymentActivate, Release: "one", Reference: "event-3"})
	assert.Equal(t, "event-3", rollback.Reference)
	assert.False(t, rollback.Unchanged)
	assert.Equal(t, "releases/one", active())
	repeated := deploy(sshRolloutInput{Action: sshDeploymentActivate, Release: "one", Reference: "event-4"})
	assert.True(t, repeated.Unchanged)
	assert.Equal(t, "event-3", repeated.Reference)

	logs := runLocalDeploymentCommand(t, "logs", sshDeploymentLogsInput{Root: root, Release: "one"})
	assert.Equal(t, "helper ran\n", string(logs))

	var pruned DeploymentPruneResult
	output = runLocalDeploymentCommand(t, "prune", map[string]any{"root": root, "keep": 1, "dry_run": false})
	require.NoError(t, json.Unmarshal(output, &pruned))
	assert.Equal(t, []Deployment{{Reference: "two"}}, pruned.Deployments)
	assert.Empty(t, pruned.Artifacts, "the retained release still uses the artifact")
	assert.NoDirExists(t, filepath.Join(root, "releases", "two"))
	assert.DirExists(t, filepath.Join(root, "releases", "one"))
	assert.Equal(t, "releases/one", active())
}

func deploymentTestArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(compressed)
	for name, content := range files {
		require.NoError(t, archive.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}))
		_, err := archive.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	require.NoError(t, compressed.Close())
	return buffer.Bytes()
}

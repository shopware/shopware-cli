package deployment

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ci"
)

func TestSSHDeploymentAcceptsEquivalentCurrentLinks(t *testing.T) {
	for _, spelling := range []string{"relative dot", "relative parent", "absolute alias"} {
		t.Run(spelling, func(t *testing.T) {
			s := localDeploymentSSH(t)
			root := filepath.Dir(s.directory)
			archive := deploymentTestArchive(t, "")
			_, err := s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, nil)
			require.NoError(t, err)
			alias := filepath.Join(t.TempDir(), "alias")
			require.NoError(t, os.Symlink(root, alias))
			setCurrent := func(name string) {
				target := "./releases/" + name
				switch spelling {
				case "relative parent":
					target = "releases/../releases/" + name
				case "absolute alias":
					target = filepath.Join(alias, "releases", name)
				}
				require.NoError(t, os.Remove(s.directory))
				require.NoError(t, os.Symlink(target, s.directory))
			}
			setCurrent("first")
			candidates, err := s.RollbackCandidates(t.Context())
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			assert.True(t, candidates[0].Active)
			unchanged, err := s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, nil)
			require.NoError(t, err)
			assert.True(t, unchanged.Unchanged)
			_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "second"}, archive, nil)
			require.NoError(t, err, "existing rollout must accept equivalent safe current links")
			setCurrent("second")
			_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, nil)
			require.NoError(t, err)
			assert.Equal(t, "releases/first", currentRelease(t, s))
		})
	}
}

func TestSSHRetainedActivation(t *testing.T) {
	s := localDeploymentSSH(t)
	root := filepath.Dir(s.directory)
	candidates, err := s.RollbackCandidates(t.Context())
	require.NoError(t, err)
	assert.Empty(t, candidates)
	assert.NoDirExists(t, filepath.Join(root, ".shopware-cli"), "discovery is read-only")
	archive := deploymentTestArchive(t, `echo 'retained helper log'; file_put_contents('helper-runs', 'once', FILE_APPEND);`)
	first, err := s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, nil)
	require.NoError(t, err)
	_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "second"}, archive, nil)
	require.NoError(t, err)
	require.NoError(t, os.Remove(archive))
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".shopware-cli/artifacts")))
	// Remote-only activation must not depend on a local workspace either.
	s.root = filepath.Join(t.TempDir(), "missing-checkout")

	candidates, err = s.RollbackCandidates(t.Context())
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	assert.Equal(t, "second", candidates[0].Deployment.Reference)
	assert.True(t, candidates[0].Active)
	assert.False(t, candidates[1].Active)
	require.NotNil(t, candidates[0].CreatedAt)
	require.NotNil(t, candidates[1].CreatedAt)
	secondCreated, firstCreated := *candidates[0].CreatedAt, *candidates[1].CreatedAt
	firstTime := *candidates[1].DeployedAt
	var output bytes.Buffer
	activated, err := s.ActivateDeployment(t.Context(), candidates[1].Deployment, &output)
	require.NoError(t, err, output.String())
	assert.NotEqual(t, first.Reference, activated.Reference)
	require.NotNil(t, activated.DeployedAt)
	assert.True(t, activated.DeployedAt.After(firstTime))
	assert.Equal(t, "releases/first", currentRelease(t, s))
	assert.NotContains(t, output.String(), "Uploading")
	assert.NotContains(t, output.String(), "Preparing release")
	assert.NotContains(t, output.String(), "retained helper log")
	assert.NoDirExists(t, filepath.Join(root, ".shopware-cli/artifacts"))

	unchanged, err := s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, nil)
	require.NoError(t, err)
	assert.True(t, unchanged.Unchanged)
	assert.Equal(t, activated.Reference, unchanged.Reference)
	assert.Equal(t, activated.DeployedAt, unchanged.DeployedAt)
	candidates, err = s.RollbackCandidates(t.Context())
	require.NoError(t, err)
	require.Len(t, candidates, 2, "multiple activations deduplicate by release")
	assert.Equal(t, "second", candidates[0].Deployment.Reference)
	assert.Equal(t, "first", candidates[1].Deployment.Reference)
	assert.False(t, candidates[0].Active)
	assert.True(t, candidates[1].Active)
	assert.Equal(t, &secondCreated, candidates[0].CreatedAt)
	assert.Equal(t, &firstCreated, candidates[1].CreatedAt)

	_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "second"}, nil)
	require.NoError(t, err)
	history, err := s.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, history, 4)
	assert.True(t, history[0].Active)
	for _, event := range history[1:] {
		assert.False(t, event.Active)
	}
	for _, name := range []string{"first", "second"} {
		marker, err := os.ReadFile(filepath.Join(root, "releases", name, "helper-runs"))
		require.NoError(t, err)
		assert.Equal(t, "once", string(marker))
		log, err := os.ReadFile(filepath.Join(root, ".shopware-cli/logs", name+".log"))
		require.NoError(t, err)
		assert.Equal(t, "retained helper log", string(log))
	}
	oldMetadata, err := os.ReadFile(filepath.Join(root, ".shopware-cli/rollouts", first.Reference+".json"))
	require.NoError(t, err)
	newMetadata, err := os.ReadFile(filepath.Join(root, ".shopware-cli/rollouts", activated.Reference+".json"))
	require.NoError(t, err)
	var oldEvent, newEvent map[string]any
	require.NoError(t, json.Unmarshal(oldMetadata, &oldEvent))
	require.NoError(t, json.Unmarshal(newMetadata, &newEvent))
	for _, key := range []string{"deployment", "archive", "sha256"} {
		assert.Equal(t, oldEvent[key], newEvent[key])
	}
	_, err = s.PruneDeployments(t.Context(), DeploymentPruneOptions{Keep: 0})
	require.NoError(t, err)
	candidates, err = s.RollbackCandidates(t.Context())
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, "second", candidates[0].Deployment.Reference)
	_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, nil)
	require.Error(t, err, "pruned releases cannot be resurrected")
}

func TestRollbackCandidatesRecoverStableCreationDate(t *testing.T) {
	s := localDeploymentSSH(t)
	root := filepath.Dir(s.directory)
	archive := deploymentTestArchive(t, "")
	for _, name := range []string{"first", "second"} {
		_, err := s.rolloutArchive(t.Context(), Deployment{Reference: name}, archive, nil)
		require.NoError(t, err)
	}
	before, err := s.RollbackCandidates(t.Context())
	require.NoError(t, err)
	require.Len(t, before, 2)
	stateFile := filepath.Join(root, ".shopware-cli/releases/first.json")
	data, err := os.ReadFile(stateFile)
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, json.Unmarshal(data, &state))
	delete(state, "created_at") // Simulate an existing release from before this field.
	data, err = json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(stateFile, data, 0o600))

	recovered, err := s.RollbackCandidates(t.Context())
	require.NoError(t, err)
	assert.Equal(t, before, recovered, "discovery must recover the original date without writing metadata")
	unchanged, err := os.ReadFile(stateFile)
	require.NoError(t, err)
	assert.Equal(t, data, unchanged)
	_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, nil)
	require.NoError(t, err)
	after, err := s.RollbackCandidates(t.Context())
	require.NoError(t, err)
	require.Len(t, after, 2)
	for i := range before {
		assert.Equal(t, before[i].Deployment, after[i].Deployment)
		assert.Equal(t, before[i].CreatedAt, after[i].CreatedAt)
	}
	assert.True(t, after[1].Active)
	assert.True(t, after[1].DeployedAt.After(*before[1].DeployedAt), "activation history must still get a new timestamp")
}

func TestSSHRetainedEligibility(t *testing.T) {
	for _, damage := range []string{"missing", "pruning", "pending", "incomplete", "failed", "checksum", "symlink", "legacy", "timestamp"} {
		t.Run(damage, func(t *testing.T) {
			s := localDeploymentSSH(t)
			root := filepath.Dir(s.directory)
			archive := deploymentTestArchive(t, "")
			first, err := s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, nil)
			require.NoError(t, err)
			_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "second"}, archive, nil)
			require.NoError(t, err)
			statePath := filepath.Join(root, ".shopware-cli/releases/first.json")
			eventPath := filepath.Join(root, ".shopware-cli/rollouts", first.Reference+".json")
			switch damage {
			case "missing":
				require.NoError(t, os.RemoveAll(filepath.Join(root, "releases/first")))
			case "symlink":
				require.NoError(t, os.RemoveAll(filepath.Join(root, "releases/first")))
				require.NoError(t, os.Symlink("second", filepath.Join(root, "releases/first")))
			case "legacy":
				require.NoError(t, os.Remove(statePath))
			default:
				file := statePath
				if damage == "failed" {
					file = eventPath
				}
				data, err := os.ReadFile(file)
				require.NoError(t, err)
				var record map[string]any
				require.NoError(t, json.Unmarshal(data, &record))
				switch damage {
				case "pruning":
					record["pruning"] = true
				case "pending":
					record["activation_pending"] = "unfinished"
				case "incomplete":
					record["ready"] = false
				case "failed":
					record["status"] = "failed"
				case "checksum":
					record["sha256"] = strings.Repeat("0", 64)
				case "timestamp":
					record["deployed_at"] = "tomorrow"
				}
				data, err = json.Marshal(record)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(file, data, 0o600))
			}
			candidates, err := s.RollbackCandidates(t.Context())
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			assert.Equal(t, "second", candidates[0].Deployment.Reference)
			_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, nil)
			require.Error(t, err)
			assert.Equal(t, "releases/second", currentRelease(t, s))
			// The same corruption in current must error, never silently omit it.
			require.NoError(t, os.Remove(s.directory))
			require.NoError(t, os.Symlink("releases/first", s.directory))
			_, err = s.RollbackCandidates(t.Context())
			require.Error(t, err)
			_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "second"}, nil)
			require.Error(t, err)
		})
	}
}

func TestSSHActivationProtocol(t *testing.T) {
	for _, action := range []string{"UPLOAD", "CACHED"} {
		var stdin bytes.Buffer
		_, err := exchangeSSHDeployment(t.Context(), &stdin,
			bufio.NewReader(strings.NewReader(action+" id\n")), nil,
			sshRolloutInput{Reference: "id", Action: "activate"}, ci.New(io.Discard))
		require.ErrorContains(t, err, "unexpected archive operation")
		assert.Empty(t, stdin.String())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdin bytes.Buffer
	_, err := exchangeSSHDeployment(ctx, &stdin, bufio.NewReader(strings.NewReader("REUSE id\nREADY id\n")),
		nil, sshRolloutInput{Reference: "id", Action: "activate"}, ci.New(io.Discard))
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, stdin.String())
}

func TestSSHActivationDisconnectAndLock(t *testing.T) {
	s := localDeploymentSSH(t)
	root := filepath.Dir(s.directory)
	archive := deploymentTestArchive(t, "")
	_, err := s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, nil)
	require.NoError(t, err)
	_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "second"}, archive, nil)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	input, err := json.Marshal(sshRolloutInput{Action: "activate", Root: root, Reference: "disconnected", Release: "first"})
	require.NoError(t, err)
	cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(input)).Cmd
	stdin, err := cmd.StdinPipe()
	require.NoError(t, err)
	defer func() { _ = stdin.Close() }()
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	defer func() { _ = stdout.Close() }()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Start())
	reader := bufio.NewReader(stdout)
	require.NoError(t, readRolloutMessage(reader, "REUSE disconnected"))
	require.NoError(t, readRolloutMessage(reader, "READY disconnected"))
	// The prepared activation owns the same lock as every other mutation.
	_, err = s.ActivateDeployment(ctx, Deployment{Reference: "first"}, nil)
	require.Error(t, err)
	_, err = s.RollbackCandidates(ctx)
	require.ErrorContains(t, err, "holds the lock")
	_, err = s.PruneDeployments(ctx, DeploymentPruneOptions{Keep: 1})
	require.ErrorContains(t, err, "holds the lock")
	_, err = s.rolloutArchive(ctx, Deployment{Reference: "third"}, archive, nil)
	require.Error(t, err)
	require.NoError(t, stdin.Close())
	require.Error(t, cmd.Wait())
	assert.Contains(t, stderr.String(), "did not authorize activation")
	assert.Equal(t, "releases/second", currentRelease(t, s))
	history, err := s.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, history, 2)
	// A failed attempt did not alter the original successful release state.
	_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, nil)
	require.NoError(t, err)
}

func TestSSHActivationUnsafeStorage(t *testing.T) {
	for _, target := range []string{"current-directory", "current-outside", "rollouts", "releases", "deployment.lock"} {
		t.Run(target, func(t *testing.T) {
			s := localDeploymentSSH(t)
			root := filepath.Dir(s.directory)
			_, err := s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, deploymentTestArchive(t, ""), nil)
			require.NoError(t, err)
			switch target {
			case "current-directory":
				require.NoError(t, os.Remove(s.directory))
				require.NoError(t, os.Mkdir(s.directory, 0o755))
			case "current-outside":
				require.NoError(t, os.Remove(s.directory))
				require.NoError(t, os.Symlink(t.TempDir(), s.directory))
			default:
				file := filepath.Join(root, ".shopware-cli", target)
				require.NoError(t, os.Rename(file, file+".saved"))
				require.NoError(t, os.Symlink(file+".saved", file))
			}
			_, err = s.RollbackCandidates(t.Context())
			require.Error(t, err)
			_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, nil)
			require.Error(t, err)
		})
	}
}

package ssh

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
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

func runDeploymentPermissions(t *testing.T, root, script string) (string, error) {
	t.Helper()
	functions, _, found := strings.Cut(sshDeploymentMainScript, "$upload = null;")
	require.True(t, found)
	code := strings.TrimPrefix(sshPermissionsScript, "<?php\n") + "\n" +
		strings.TrimPrefix(functions, "<?php\n") + "\n" +
		`$root = $argv[1]; $release = "$root/releases/test"; ` + script
	output, err := exec.CommandContext(t.Context(), deploymentPHP(t), "-r", code, "--", root).CombinedOutput()
	return string(output), err
}

func deploymentPermissionMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

func TestSSHPermissionsSealPreservesAccessAndRuntimeWrites(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() { restoreDeploymentDirectories(t, root) })
	release := filepath.Join(root, "releases/test")
	for name, mode := range map[string]os.FileMode{
		"regular.php": 0o644, "bin/console": 0o755, "private.key": 0o600,
		"private-executable": 0o700, "var/cache/existing": 0o660,
		"readonly-dir/file": 0o640,
	} {
		path := filepath.Join(release, name)
		testhelper.WriteFile(t, path, name)
		require.NoError(t, os.Chmod(path, mode))
	}
	require.NoError(t, os.Chmod(filepath.Join(release, "readonly-dir"), 0o550))
	require.NoError(t, os.Chmod(filepath.Join(release, "var/cache"), 0o770))
	require.NoError(t, os.Chmod(filepath.Join(release, "var"), 0o555))
	external := filepath.Join(root, "shared/private")
	testhelper.WriteFile(t, external, "shared")
	require.NoError(t, os.Chmod(external, 0o600))
	require.NoError(t, os.Symlink(external, filepath.Join(release, "shared-link")))
	require.NoError(t, os.Symlink(external, filepath.Join(release, "var/cache/cache-link")))
	require.NoError(t, os.Symlink(filepath.Join(root, "absent"), filepath.Join(release, "dangling-link")))

	output, err := runDeploymentPermissions(t, root, `
deploymentRuntimeCache($root, $release, 'test');
deploymentSealRelease($root, $release, 'test');
deploymentVerifySealedRelease($root, $release, 'test');
`)
	require.NoError(t, err, output)
	for name, mode := range map[string]os.FileMode{
		"regular.php": 0o444, "bin/console": 0o555, "private.key": 0o400,
		"private-executable": 0o500, "readonly-dir": 0o550,
		"readonly-dir/file": 0o440, "var": 0o555,
		"var/cache": 0o770, "var/cache/existing": 0o660,
	} {
		assert.Equal(t, mode, deploymentPermissionMode(t, filepath.Join(release, name)), name)
	}
	assert.Zero(t, deploymentPermissionMode(t, release)&0o222)
	assert.Equal(t, os.FileMode(0o600), deploymentPermissionMode(t, external))
	cache, err := os.Readlink(filepath.Join(release, "var/cache"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "runtime/test/cache"), cache)
	require.NoError(t, os.WriteFile(filepath.Join(release, "var/cache/new"), []byte("runtime"), 0o600))
	data, err := os.ReadFile(filepath.Join(root, "runtime/test/cache/new"))
	require.NoError(t, err)
	assert.Equal(t, "runtime", string(data))
	if os.Geteuid() != 0 {
		assert.Error(t, os.WriteFile(filepath.Join(release, "regular.php"), []byte("modified"), 0o644))
		assert.Error(t, os.WriteFile(filepath.Join(release, "new-file"), []byte("modified"), 0o644))
	}

	// Rechecking a sealed release leaves links, permissions and cache unchanged.
	output, err = runDeploymentPermissions(t, root, `
deploymentRuntimeCache($root, $release, 'test', true);
deploymentSealRelease($root, $release, 'test');
`)
	require.NoError(t, err, output)
	assert.Equal(t, os.FileMode(0o600), deploymentPermissionMode(t, external))
}

func TestSSHPermissionsRejectUnsafeRuntimePaths(t *testing.T) {
	for _, path := range []string{"runtime", "runtime/test", "runtime/test/cache", "releases/test/var", "releases/test/var/cache"} {
		t.Run(path, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "outside")
			require.NoError(t, os.MkdirAll(target, 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "releases/test"), 0o755))
			link := filepath.Join(root, path)
			require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
			require.NoError(t, os.Symlink(target, link))
			output, err := runDeploymentPermissions(t, root, `deploymentRuntimeCache($root, $release, 'test');`)
			require.Error(t, err, output)
			assert.Equal(t, os.FileMode(0o755), deploymentPermissionMode(t, target))
			entries, err := os.ReadDir(target)
			require.NoError(t, err)
			assert.Empty(t, entries)
		})
	}
	t.Run("archive managed link is still forbidden", func(t *testing.T) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "releases/test/var"), 0o755))
		cache := filepath.Join(root, "runtime/test/cache")
		require.NoError(t, os.MkdirAll(cache, 0o755))
		require.NoError(t, os.Symlink(cache, filepath.Join(root, "releases/test/var/cache")))
		output, err := runDeploymentPermissions(t, root, `deploymentRuntimeCache($root, $release, 'test');`)
		require.Error(t, err)
		assert.Contains(t, output, "Archive cache must not be a symlink")
	})
}

func TestSSHPermissionsRuntimeDirectoriesGainOwnerAccessOnly(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() { restoreDeploymentDirectories(t, root) })
	release := filepath.Join(root, "releases/test")
	testhelper.WriteFile(t, filepath.Join(release, "var/cache/private/entry"), "cached")
	require.NoError(t, os.Chmod(filepath.Join(release, "var/cache/private/entry"), 0o400))
	require.NoError(t, os.Chmod(filepath.Join(release, "var/cache/private"), 0o500))
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.Mkdir(outside, 0o550))
	require.NoError(t, os.Symlink(outside, filepath.Join(release, "var/cache/link")))
	require.NoError(t, os.Chmod(filepath.Join(release, "var/cache"), 0o550))

	output, err := runDeploymentPermissions(t, root, `
deploymentRuntimeCache($root, $release, 'test');
deploymentSealRelease($root, $release, 'test');
deploymentVerifySealedRelease($root, $release, 'test');
`)
	require.NoError(t, err, output)
	assert.Equal(t, os.FileMode(0o750), deploymentPermissionMode(t, filepath.Join(release, "var/cache")))
	assert.Equal(t, os.FileMode(0o700), deploymentPermissionMode(t, filepath.Join(release, "var/cache/private")))
	assert.Equal(t, os.FileMode(0o400), deploymentPermissionMode(t, filepath.Join(release, "var/cache/private/entry")))
	assert.Equal(t, os.FileMode(0o550), deploymentPermissionMode(t, outside))
	require.NoError(t, os.WriteFile(filepath.Join(release, "var/cache/private/new"), []byte("runtime"), 0o600))
}

func TestSSHPermissionsVerifyRuntimeAccessWithoutRepairingActiveRelease(t *testing.T) {
	for _, directory := range []string{"runtime", "runtime/test", "runtime/test/cache", "runtime/test/cache/nested"} {
		t.Run(directory, func(t *testing.T) {
			root := t.TempDir()
			t.Cleanup(func() { restoreDeploymentDirectories(t, root) })
			testhelper.WriteFile(t, filepath.Join(root, "releases/test/var/cache/nested/entry"), "cached")
			output, err := runDeploymentPermissions(t, root, `
deploymentRuntimeCache($root, $release, 'test');
deploymentSealRelease($root, $release, 'test');
`)
			require.NoError(t, err, output)
			path := filepath.Join(root, directory)
			require.NoError(t, os.Chmod(path, 0o555))
			output, err = runDeploymentPermissions(t, root, `deploymentVerifySealedRelease($root, $release, 'test');`)
			require.Error(t, err)
			assert.Contains(t, output, "Runtime directory must be writable")
			assert.Equal(t, os.FileMode(0o555), deploymentPermissionMode(t, path))

			output, err = runDeploymentPermissions(t, root, `
deploymentRuntimeCache($root, $release, 'test', true);
deploymentSealRelease($root, $release, 'test');
deploymentVerifySealedRelease($root, $release, 'test');
`)
			require.NoError(t, err, output)
			assert.Equal(t, os.FileMode(0o755), deploymentPermissionMode(t, path))
		})
	}
}

func TestSSHPermissionsNoopDoesNotBootstrapDirectories(t *testing.T) {
	for _, damage := range []string{"missing auxiliary directories", "writable release", "missing lock"} {
		t.Run(damage, func(t *testing.T) {
			s := localDeploymentSSH(t)
			root := filepath.Dir(s.directory)
			archive := deploymentTestArchive(t, "")
			artifact := Deployment{Reference: "first"}
			first, err := s.rolloutArchive(t.Context(), artifact, archive, nil)
			require.NoError(t, err)
			require.NoError(t, os.RemoveAll(filepath.Join(root, "shared")))
			require.NoError(t, os.RemoveAll(filepath.Join(root, ".shopware-cli/artifacts")))
			switch damage {
			case "writable release":
				require.NoError(t, os.Chmod(filepath.Join(root, "releases/first"), 0o755))
			case "missing lock":
				require.NoError(t, os.Remove(filepath.Join(root, ".shopware-cli/deployment.lock")))
			}
			for _, rollback := range []bool{false, true} {
				var result Rollout
				if rollback {
					result, err = s.ActivateDeployment(t.Context(), artifact, nil)
				} else {
					result, err = s.rolloutArchive(t.Context(), artifact, archive, nil)
				}
				if damage == "missing auxiliary directories" {
					require.NoError(t, err)
					assert.True(t, result.Unchanged)
					assert.Equal(t, first.Reference, result.Reference)
				} else {
					require.Error(t, err)
				}
				assert.NoDirExists(t, filepath.Join(root, "shared"))
				assert.NoDirExists(t, filepath.Join(root, ".shopware-cli/artifacts"))
				if damage == "missing lock" {
					assert.NoFileExists(t, filepath.Join(root, ".shopware-cli/deployment.lock"))
				}
			}
		})
	}
}

func TestSSHPermissionsRecoverInterruptedCacheMove(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() { restoreDeploymentDirectories(t, root) })
	require.NoError(t, os.MkdirAll(filepath.Join(root, "releases/test"), 0o755))
	require.NoError(t, os.Chmod(filepath.Join(root, "releases/test"), 0o555))
	testhelper.WriteFile(t, filepath.Join(root, "runtime/test/cache/existing"), "preserved")
	output, err := runDeploymentPermissions(t, root, `
deploymentRuntimeCache($root, $release, 'test', true);
deploymentSealRelease($root, $release, 'test');
`)
	require.NoError(t, err, output)
	data, err := os.ReadFile(filepath.Join(root, "releases/test/var/cache/existing"))
	require.NoError(t, err)
	assert.Equal(t, "preserved", string(data))
}

func TestSSHPermissionsInactiveLegacyReleaseSealedWithoutHelper(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		name := "reuse"
		if rollback {
			name = "rollback"
		}
		t.Run(name, func(t *testing.T) {
			s := localDeploymentSSH(t)
			archive := deploymentTestArchive(t, `file_put_contents('var/cache/helper-count', 'once');`)
			_, err := s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, io.Discard)
			require.NoError(t, err)
			_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "second"}, archive, io.Discard)
			require.NoError(t, err)
			root := filepath.Dir(s.directory)
			release := filepath.Join(root, "releases/first")
			// Model an older retained release with a real cache and writable code.
			restoreDeploymentDirectories(t, release)
			require.NoError(t, os.Chmod(filepath.Join(release, "composer.json"), 0o644))
			require.NoError(t, os.Remove(filepath.Join(release, "var/cache")))
			require.NoError(t, os.Rename(filepath.Join(root, "runtime/first/cache"), filepath.Join(release, "var/cache")))
			logBefore, err := os.ReadFile(filepath.Join(root, ".shopware-cli/logs/first.log"))
			require.NoError(t, err)
			candidates, err := s.RollbackCandidates(t.Context())
			require.NoError(t, err)
			require.Len(t, candidates, 2, "discovery must accept older unsealed releases")
			assert.Equal(t, os.FileMode(0o644), deploymentPermissionMode(t, filepath.Join(release, "composer.json")))

			if rollback {
				_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, io.Discard)
			} else {
				_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, io.Discard)
			}
			require.NoError(t, err)
			assert.Equal(t, "releases/first", currentRelease(t, s))
			assert.Equal(t, os.FileMode(0o444), deploymentPermissionMode(t, filepath.Join(release, "composer.json")))
			cache, err := os.Readlink(filepath.Join(release, "var/cache"))
			require.NoError(t, err)
			assert.Equal(t, filepath.Join(root, "runtime/first/cache"), cache)
			logAfter, err := os.ReadFile(filepath.Join(root, ".shopware-cli/logs/first.log"))
			require.NoError(t, err)
			assert.Equal(t, logBefore, logAfter, "retained release must not rerun helper")
		})
	}
}

func TestSSHPermissionsActiveNoopRejectsUnprotectedRelease(t *testing.T) {
	for _, legacyCache := range []bool{false, true} {
		name := "writable-code"
		if legacyCache {
			name = "legacy-cache"
		}
		t.Run(name, func(t *testing.T) {
			s := localDeploymentSSH(t)
			archive := deploymentTestArchive(t, "")
			_, err := s.rolloutArchive(t.Context(), Deployment{Reference: "active"}, archive, io.Discard)
			require.NoError(t, err)
			root := filepath.Dir(s.directory)
			release := filepath.Join(root, "releases/active")
			if legacyCache {
				varPath := filepath.Join(release, "var")
				require.NoError(t, os.Chmod(varPath, 0o755))
				require.NoError(t, os.Remove(filepath.Join(varPath, "cache")))
				require.NoError(t, os.Rename(filepath.Join(root, "runtime/active/cache"), filepath.Join(varPath, "cache")))
				require.NoError(t, os.Chmod(varPath, 0o555))
			} else {
				require.NoError(t, os.Chmod(filepath.Join(release, "composer.json"), 0o644))
			}
			metadataPath := filepath.Join(root, ".shopware-cli/releases/active.json")
			before, err := os.ReadFile(metadataPath)
			require.NoError(t, err)
			linkBefore, err := os.Lstat(s.directory)
			require.NoError(t, err)
			var logs bytes.Buffer
			_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "active"}, archive, &logs)
			require.Error(t, err)
			assert.Contains(t, logs.String(), "deploy a new release name")
			logs.Reset()
			_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "active"}, &logs)
			require.Error(t, err)
			assert.Contains(t, logs.String(), "deploy a new release name")
			after, err := os.ReadFile(metadataPath)
			require.NoError(t, err)
			assert.Equal(t, before, after)
			linkAfter, err := os.Lstat(s.directory)
			require.NoError(t, err)
			assert.True(t, os.SameFile(linkBefore, linkAfter))
			if !legacyCache {
				assert.Equal(t, os.FileMode(0o644), deploymentPermissionMode(t, filepath.Join(release, "composer.json")))
			} else {
				info, err := os.Lstat(filepath.Join(release, "var/cache"))
				require.NoError(t, err)
				assert.True(t, info.IsDir(), "no-op must not migrate an active cache")
			}
		})
	}
}

func TestSSHPermissionsRejectSpecialFiles(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("mkfifo is required")
	}
	for _, runtimeCache := range []bool{false, true} {
		name := "code"
		if runtimeCache {
			name = "cache"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Cleanup(func() { restoreDeploymentDirectories(t, root) })
			release := filepath.Join(root, "releases/test")
			require.NoError(t, os.MkdirAll(filepath.Join(release, "var/cache"), 0o755))
			fifo := filepath.Join(release, "special")
			if runtimeCache {
				fifo = filepath.Join(release, "var/cache/special")
			}
			require.NoError(t, exec.CommandContext(t.Context(), mkfifo, fifo).Run())
			output, err := runDeploymentPermissions(t, root, `
deploymentRuntimeCache($root, $release, 'test');
deploymentSealRelease($root, $release, 'test');
`)
			require.Error(t, err)
			assert.Contains(t, output, "special file")
		})
	}
}

func TestSSHPermissionsSealedBeforeReady(t *testing.T) {
	for _, coordinated := range []bool{false, true} {
		name := "singlehost"
		if coordinated {
			name = "coordinated"
		}
		t.Run(name, func(t *testing.T) {
			s := localDeploymentSSH(t)
			archive := deploymentTestArchive(t, "")
			_, err := s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, io.Discard)
			require.NoError(t, err)
			_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "second"}, archive, io.Discard)
			require.NoError(t, err)
			root := filepath.Dir(s.directory)
			release := filepath.Join(root, "releases/first")
			require.NoError(t, os.Chmod(filepath.Join(release, "composer.json"), 0o644))
			metadataFile := filepath.Join(root, ".shopware-cli/releases/first.json")
			data, err := os.ReadFile(metadataFile)
			require.NoError(t, err)
			var state map[string]any
			require.NoError(t, json.Unmarshal(data, &state))
			payload, err := json.Marshal(map[string]any{
				"action": "activate", "root": root, "reference": "permission-check",
				"release": "first", "coordinated": coordinated, "sha256": state["sha256"],
			})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			cmd := s.transport.RemotePHPCommand(ctx, "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload)).Cmd
			stdin, err := cmd.StdinPipe()
			require.NoError(t, err)
			defer func() { _ = stdin.Close() }()
			stdout, err := cmd.StdoutPipe()
			require.NoError(t, err)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			require.NoError(t, cmd.Start())
			reader := bufio.NewReader(stdout)
			require.NoError(t, readRolloutMessage(reader, "REUSE permission-check"))
			if coordinated {
				require.NoError(t, readRolloutMessage(reader, "STAGED permission-check"))
				assert.Equal(t, os.FileMode(0o644), deploymentPermissionMode(t, filepath.Join(release, "composer.json")))
				_, err = io.WriteString(stdin, "PREPARE permission-check\n")
				require.NoError(t, err)
			}
			require.NoError(t, readRolloutMessage(reader, "READY permission-check"))
			assert.Equal(t, os.FileMode(0o444), deploymentPermissionMode(t, filepath.Join(release, "composer.json")))
			data, err = os.ReadFile(metadataFile)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(data, &state))
			assert.Equal(t, true, state["ready"])
			assert.Equal(t, "releases/second", currentRelease(t, s))
			require.NoError(t, stdin.Close())
			require.Error(t, cmd.Wait(), stderr.String())
			assert.Equal(t, "releases/second", currentRelease(t, s), "disconnect must not activate")
			// Disconnection after sealing is safe to retry without rerunning helper.
			_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, io.Discard)
			require.NoError(t, err)
		})
	}
}

func TestSSHPermissionsFailedInactiveSealingClearsReadiness(t *testing.T) {
	s := localDeploymentSSH(t)
	archive := deploymentTestArchive(t, "")
	_, err := s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, io.Discard)
	require.NoError(t, err)
	_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "second"}, archive, io.Discard)
	require.NoError(t, err)
	root := filepath.Dir(s.directory)
	varPath := filepath.Join(root, "releases/first/var")
	require.NoError(t, os.Chmod(varPath, 0o755))
	require.NoError(t, os.Remove(filepath.Join(varPath, "cache")))
	require.NoError(t, os.Symlink(filepath.Join(root, "shared"), filepath.Join(varPath, "cache")))
	_, err = s.ActivateDeployment(t.Context(), Deployment{Reference: "first"}, io.Discard)
	require.Error(t, err)
	data, err := os.ReadFile(filepath.Join(root, ".shopware-cli/releases/first.json"))
	require.NoError(t, err)
	var state map[string]any
	require.NoError(t, json.Unmarshal(data, &state))
	assert.Equal(t, false, state["ready"])
	assert.Equal(t, "releases/second", currentRelease(t, s))
	// Even after repairing the link, stale successful metadata cannot authorize
	// reuse of a release whose previous preparation may have been interrupted.
	require.NoError(t, os.Remove(filepath.Join(varPath, "cache")))
	require.NoError(t, os.Symlink(filepath.Join(root, "runtime/first/cache"), filepath.Join(varPath, "cache")))
	var logs bytes.Buffer
	_, err = s.rolloutArchive(t.Context(), Deployment{Reference: "first"}, archive, &logs)
	require.Error(t, err)
	assert.Contains(t, logs.String(), "incomplete preparation")
}

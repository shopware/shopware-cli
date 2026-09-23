package deployment

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/ci"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func deploymentPHP(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("SSH release layout requires a POSIX host")
	}
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP CLI is required to execute the remote deployment script")
	}
	if _, err := exec.LookPath("tar"); err != nil {
		t.Skip("tar is required")
	}
	return php
}

func localDeploymentSSH(t *testing.T) *SSH {
	t.Helper()
	php := deploymentPHP(t)
	t.Setenv("DATABASE_URL", "")
	bin := t.TempDir()
	// Execute the SSH remote command locally. No network or sshd is involved.
	testhelper.WriteFile(t, filepath.Join(bin, "ssh"), "#!/bin/sh\nif [ -n \"$SSH_ARGS_LOG\" ]; then printf '%s\\n' \"$@\" > \"$SSH_ARGS_LOG\"; fi\nfor arg do command=\"$arg\"; done\nexec /bin/sh -c \"$command\"\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "ssh"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// Spaces and apostrophes must survive SSH's remote shell.
	root = filepath.Join(root, "shop's releases")
	testhelper.WriteFile(t, filepath.Join(root, "shared/.env.local"), "DATABASE_URL=provided-on-server\n")
	testhelper.WriteFile(t, filepath.Join(root, "shared/public/media/upload"), "persistent")
	backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh",
		SSH:  &shop.EnvironmentSSHConfig{Host: "example.invalid", Directory: filepath.Join(root, "current"), PHPBinary: php},
	})
	require.NoError(t, err)
	return backend.(*SSH)
}

func deploymentTestArchive(t *testing.T, helper string) string {
	t.Helper()
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), "{}")
	testhelper.WriteFile(t, filepath.Join(root, "vendor/bin/shopware-deployment-helper"), "<?php\n"+helper)
	testhelper.WriteFile(t, filepath.Join(root, "public/bundles/app.js"), "built")
	testhelper.WriteFile(t, filepath.Join(root, "config/jwt/test.key"), "fixture-key")
	file := filepath.Join(t.TempDir(), "shop.tar.gz")
	out, err := os.Create(file)
	require.NoError(t, err)
	require.NoError(t, archiver.WriteTarGz(t.Context(), out, root, nil))
	require.NoError(t, out.Close())
	return file
}

func currentRelease(t *testing.T, e *SSH) string {
	t.Helper()
	target, err := os.Readlink(e.directory)
	require.NoError(t, err)
	return target
}

func TestSSHRolloutPreservesConfiguredPHPWrapper(t *testing.T) {
	e := localDeploymentSSH(t)
	bin := t.TempDir()
	wrapper := filepath.Join(bin, "custom-php")
	calls := filepath.Join(bin, "calls")
	t.Setenv("DEPLOYMENT_TEST_PHP", deploymentPHP(t))
	t.Setenv("DEPLOYMENT_TEST_PHP_CALLS", calls)
	testhelper.WriteFile(t, wrapper, `#!/bin/sh
printf '%s\n' "$1" >> "$DEPLOYMENT_TEST_PHP_CALLS"
exec "$DEPLOYMENT_TEST_PHP" -d memory_limit=192M "$@"
`)
	require.NoError(t, os.Chmod(wrapper, 0o755))
	env := *e.env
	ssh := *env.SSH
	ssh.PHPBinary = wrapper
	env.SSH = &ssh
	backend, err := New(e.root, e.configPath, e.config, &env)
	require.NoError(t, err)
	archive := deploymentTestArchive(t, `if (ini_get('memory_limit') !== '192M') { exit(42); }`)
	_, err = backend.RolloutDeployment(t.Context(), Deployment{Reference: archive}, io.Discard)
	require.NoError(t, err)
	data, err := os.ReadFile(calls)
	require.NoError(t, err)
	assert.Equal(t, "-r\nvendor/bin/shopware-deployment-helper\n", string(data))
}

func TestSSHRolloutPreparesAndActivatesReleases(t *testing.T) {
	// This integration test asserts plain terminal sections, independently of CI.
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")
	e := localDeploymentSSH(t)
	archive := deploymentTestArchive(t,
		`if (!is_link('.env.local') || !is_link('public/media')) { exit(9); }
if (file_get_contents('.env') !== "APP_ENV=prod\nAPP_DEBUG=0\n") { exit(9); }
file_put_contents('helper-ran', implode(' ', array_slice($argv, 1)));
fwrite(STDOUT, "helper stdout\n");
fwrite(STDERR, "helper stderr\n");`)
	deployment := Deployment{Reference: "focused-turing"}
	var logs bytes.Buffer
	first, err := e.rolloutArchive(t.Context(), deployment, archive, &logs)
	require.NoError(t, err)
	assert.True(t, first.Active)
	assert.Equal(t, deployment, first.Deployment)
	assert.Contains(t, logs.String(), "--- Uploading deployment artifact ---")
	assert.Contains(t, logs.String(), "--- Preparing release ---")
	assert.Contains(t, logs.String(), "Running Shopware Deployment Helper")
	assert.Contains(t, logs.String(), "helper stdout")
	assert.Contains(t, logs.String(), "helper stderr")
	assert.Contains(t, logs.String(), "--- Activating release ---")
	assert.Contains(t, logs.String(), "--- Activating release finished in")
	assert.Equal(t, "releases/focused-turing", currentRelease(t, e))
	root := filepath.Dir(e.directory)
	assert.FileExists(t, filepath.Join(e.directory, "helper-ran"))
	assert.FileExists(t, filepath.Join(root, "shared/config/jwt/test.key"))
	assert.FileExists(t, filepath.Join(root, "shared/install.lock"))
	assert.NoDirExists(t, filepath.Join(root, "shared/var/cache"))

	logPath := filepath.Join(root, ".shopware-cli/logs/focused-turing.log")
	helperLogsBefore, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, "helper stdout\nhelper stderr\n", string(helperLogsBefore))
	metadataBefore, err := os.ReadFile(filepath.Join(root, ".shopware-cli/rollouts", first.Reference+".json"))
	require.NoError(t, err)
	releaseRecordPath := filepath.Join(root, ".shopware-cli/releases/focused-turing.json")
	releaseBefore, err := os.ReadFile(releaseRecordPath)
	require.NoError(t, err)
	linkBefore, err := os.Lstat(e.directory)
	require.NoError(t, err)
	// A prepared directory must not be modified by either no-op or reactivation.
	testhelper.WriteFile(t, filepath.Join(e.directory, "helper-ran"), "prepared once")
	logs.Reset()
	unchanged, err := e.rolloutArchive(t.Context(), deployment, archive, &logs)
	require.NoError(t, err)
	assert.True(t, unchanged.Unchanged)
	assert.Equal(t, first.Reference, unchanged.Reference)
	assert.Empty(t, logs.String())
	metadataAfter, err := os.ReadFile(filepath.Join(root, ".shopware-cli/rollouts", first.Reference+".json"))
	require.NoError(t, err)
	assert.Equal(t, metadataBefore, metadataAfter)
	releaseAfter, err := os.ReadFile(releaseRecordPath)
	require.NoError(t, err)
	assert.Equal(t, releaseBefore, releaseAfter)
	linkAfter, err := os.Lstat(e.directory)
	require.NoError(t, err)
	assert.True(t, os.SameFile(linkBefore, linkAfter), "no-op must not replace the current symlink")
	helperLogsAfter, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, helperLogsBefore, helperLogsAfter)
	history, err := e.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, history, 1, "a no-op must not create an activation event")

	otherDeployment := Deployment{Reference: "clever-hopper"}
	second, err := e.rolloutArchive(t.Context(), otherDeployment, archive, &logs)
	require.NoError(t, err)
	assert.NotEqual(t, first.Reference, second.Reference)
	assert.Contains(t, logs.String(), "--- Using cached deployment artifact ---")
	assert.Equal(t, "releases/clever-hopper", currentRelease(t, e))
	assert.DirExists(t, filepath.Join(root, "releases", deployment.Reference))
	rollouts, err := e.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, rollouts, 2)
	for _, rollout := range rollouts {
		require.NotNil(t, rollout.DeployedAt)
		assert.WithinDuration(t, time.Now(), *rollout.DeployedAt, time.Minute)
	}
	assert.True(t, rollouts[0].DeployedAt.After(*rollouts[1].DeployedAt))
	assert.Equal(t, []Rollout{
		{Reference: second.Reference, Deployment: otherDeployment, Active: true, DeployedAt: rollouts[0].DeployedAt},
		{Reference: first.Reference, Deployment: deployment, DeployedAt: rollouts[1].DeployedAt},
	}, rollouts)
	data, err := os.ReadFile(filepath.Join(e.directory, "public/media/upload"))
	require.NoError(t, err)
	assert.Equal(t, "persistent", string(data))
	metadata, err := os.ReadFile(filepath.Join(root, ".shopware-cli/rollouts", second.Reference+".json"))
	require.NoError(t, err)
	var record map[string]string
	require.NoError(t, json.Unmarshal(metadata, &record))
	assert.Equal(t, "successful", record["status"])
	assert.Equal(t, otherDeployment.Reference, record["deployment"])
	assert.Equal(t, otherDeployment.Reference, record["release"])
	assert.Equal(t, archive, record["archive"])
	deployedAt, err := time.Parse(time.RFC3339Nano, record["deployed_at"])
	require.NoError(t, err)
	assert.True(t, deployedAt.Equal(*rollouts[0].DeployedAt))
	createdAt, err := time.Parse(time.RFC3339Nano, record["created_at"])
	require.NoError(t, err)
	assert.True(t, deployedAt.After(createdAt), "deployment time must record activation, not preparation")
	uploads, err := filepath.Glob(filepath.Join(root, ".shopware-cli/*.tar.gz"))
	require.NoError(t, err)
	assert.Empty(t, uploads)
	artifacts, err := filepath.Glob(filepath.Join(root, ".shopware-cli/artifacts/*.tar.gz"))
	require.NoError(t, err)
	assert.Len(t, artifacts, 1)
	artifactInfo, err := os.Stat(artifacts[0])
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), artifactInfo.Mode().Perm())
	artifactDirectory, err := os.Stat(filepath.Dir(artifacts[0]))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), artifactDirectory.Mode().Perm())
	require.NoError(t, os.WriteFile(artifacts[0], []byte("damaged"), 0o600))
	logs.Reset()
	third, err := e.rolloutArchive(t.Context(), deployment, archive, &logs)
	require.NoError(t, err)
	assert.NotEqual(t, first.Reference, third.Reference, "reactivation creates a new history event, not a new directory")
	assert.Contains(t, logs.String(), "--- Reusing prepared release ---")
	assert.NotContains(t, logs.String(), "artifact")
	assert.NotContains(t, logs.String(), "Preparing release")
	assert.NotContains(t, logs.String(), "Running Shopware Deployment Helper")
	assert.Equal(t, "releases/focused-turing", currentRelease(t, e))
	helperMarker, err := os.ReadFile(filepath.Join(e.directory, "helper-ran"))
	require.NoError(t, err)
	assert.Equal(t, "prepared once", string(helperMarker))
	helperLogsAfter, err = os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, helperLogsBefore, helperLogsAfter, "reactivation must preserve the original helper logs")
	rollouts, err = e.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, rollouts, 3)
	assert.Equal(t, third.Reference, rollouts[0].Reference)
	assert.True(t, rollouts[0].Active)
	assert.False(t, rollouts[1].Active)
	assert.False(t, rollouts[2].Active, "only the latest activation of a reused directory is active")
	directories, err := os.ReadDir(filepath.Join(root, "releases"))
	require.NoError(t, err)
	require.Len(t, directories, 2)
	assert.ElementsMatch(t, []string{"focused-turing", "clever-hopper"}, []string{directories[0].Name(), directories[1].Name()})

	logs.Reset()
	_, err = e.rolloutArchive(t.Context(), Deployment{Reference: "happy-euclid"}, archive, &logs)
	require.NoError(t, err)
	assert.Contains(t, logs.String(), "--- Uploading deployment artifact ---")
	assert.NotContains(t, logs.String(), "--- Using cached deployment artifact ---")
	assert.FileExists(t, archive, "rollout must retain the local deployment archive")
	require.NoError(t, os.Remove(archive))
	rollouts, err = e.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, rollouts, 4)
	assert.Equal(t, deployment, rollouts[1].Deployment, "history must not depend on the original local archive")
}

func TestSSHListRolloutsBeforeFirstDeployment(t *testing.T) {
	rollouts, err := localDeploymentSSH(t).ListRollouts(t.Context())
	require.NoError(t, err)
	assert.Empty(t, rollouts)
}

func TestSSHListRolloutsArchiveDisplayNames(t *testing.T) {
	e := localDeploymentSSH(t)
	metadataPath := filepath.Join(filepath.Dir(e.directory), ".shopware-cli", "rollouts", "release.json")
	for _, reference := range []string{
		"/Users/shyim/Downloads/my-fancy-shop/.shopware-cli/deployments/happy-euclid.tar.gz",
		`C:\projects\shop\.shopware-cli\deployments\happy-euclid.tar.gz`,
		"./builds/happy-euclid.tar.gz",
		"happy-euclid.tar.gz",
		"happy-euclid",
	} {
		t.Run(reference, func(t *testing.T) {
			data, err := json.Marshal(map[string]string{
				"reference": "release", "deployment": reference,
				"status": "successful", "created_at": "2026-09-22T11:29:12Z",
			})
			require.NoError(t, err)
			testhelper.WriteFile(t, metadataPath, string(data))
			rollouts, err := e.ListRollouts(t.Context())
			require.NoError(t, err)
			require.Len(t, rollouts, 1)
			assert.Equal(t, reference, rollouts[0].Deployment.Reference)
			assert.Equal(t, "happy-euclid", rollouts[0].Deployment.DisplayName())
		})
	}
}

func TestSSHListRolloutsDeploymentTimestamps(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	records := []struct {
		reference, status, createdAt, deployedAt string
	}{
		{"legacy", "successful", "2026-09-22T11:29:12+00:00", ""},
		// Prepared earlier, but activated after the legacy release in the same second.
		{"new", "successful", "2026-09-22T11:28:00.000000Z", "2026-09-22T11:29:12.123456Z"},
		{"failed", "failed", "2026-09-22T11:30:00.000000Z", ""},
	}
	for _, record := range records {
		metadata := map[string]string{
			"reference": record.reference, "deployment": "focused-turing",
			"status": record.status, "created_at": record.createdAt,
		}
		if record.deployedAt != "" {
			metadata["deployed_at"] = record.deployedAt
		}
		data, err := json.Marshal(metadata)
		require.NoError(t, err)
		testhelper.WriteFile(t, filepath.Join(root, ".shopware-cli", "rollouts", record.reference+".json"), string(data))
		require.NoError(t, os.MkdirAll(filepath.Join(root, "releases", record.reference), 0o755))
	}
	require.NoError(t, os.Symlink("releases/new", e.directory))

	rollouts, err := e.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, rollouts, 2)
	assert.Equal(t, "new", rollouts[0].Reference)
	assert.True(t, rollouts[0].Active)
	require.NotNil(t, rollouts[0].DeployedAt)
	assert.Equal(t, "2026-09-22T11:29:12.123456Z", rollouts[0].DeployedAt.Format(time.RFC3339Nano))
	assert.Equal(t, "legacy", rollouts[1].Reference)
	assert.Nil(t, rollouts[1].DeployedAt)
}

func TestSSHListRolloutsRejectsSymlinkedMetadataDirectory(t *testing.T) {
	e := localDeploymentSSH(t)
	metadataRoot := filepath.Join(filepath.Dir(e.directory), ".shopware-cli")
	require.NoError(t, os.MkdirAll(metadataRoot, 0o700))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(metadataRoot, "rollouts")))

	_, err := e.ListRollouts(t.Context())
	require.ErrorContains(t, err, "Rollout metadata path must not be a symlink")
}

func TestSSHListRolloutsUsesRecordedActivation(t *testing.T) {
	for _, scenario := range []string{"clock moved backward", "pending activation", "missing history", "missing release state"} {
		t.Run(scenario, func(t *testing.T) {
			e := localDeploymentSSH(t)
			root := filepath.Dir(e.directory)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "releases/happy-euclid"), 0o755))
			require.NoError(t, os.Symlink("releases/happy-euclid", e.directory))
			const oldTime = "2026-09-22T11:29:12Z"
			const newTime = "2026-09-22T10:29:12Z"
			writeJSON := func(relative string, value any) {
				data, err := json.Marshal(value)
				require.NoError(t, err)
				testhelper.WriteFile(t, filepath.Join(root, relative), string(data))
			}
			for reference, timestamp := range map[string]string{"old": oldTime, "new": newTime} {
				status := "successful"
				if reference == "new" && (scenario == "pending activation" || scenario == "missing history") {
					status = "prepared"
				}
				writeJSON(".shopware-cli/rollouts/"+reference+".json", map[string]string{
					"reference": reference, "release": "happy-euclid", "deployment": "happy-euclid",
					"status": status, "sha256": strings.Repeat("a", 64),
					"created_at": timestamp, "deployed_at": timestamp,
				})
			}
			state := map[string]any{
				"release": "happy-euclid", "ready": true, "reference": "new",
				"sha256": strings.Repeat("a", 64), "deployed_at": newTime,
			}
			if scenario == "pending activation" {
				state["activation_pending"] = "new"
				state["reference"] = "old"
				state["deployed_at"] = oldTime
			}
			if scenario != "missing release state" {
				writeJSON(".shopware-cli/releases/happy-euclid.json", state)
			}
			rollouts, err := e.ListRollouts(t.Context())
			if scenario != "clock moved backward" {
				require.Error(t, err, "must not misidentify the old activation as current")
				assert.Nil(t, rollouts)
				return
			}
			require.NoError(t, err)
			require.Len(t, rollouts, 2)
			assert.Equal(t, "old", rollouts[0].Reference)
			assert.False(t, rollouts[0].Active)
			assert.Equal(t, "new", rollouts[1].Reference)
			assert.True(t, rollouts[1].Active)
		})
	}
}

func TestSSHRolloutFailuresKeepCurrent(t *testing.T) {
	e := localDeploymentSSH(t)
	good := deploymentTestArchive(t, "")
	_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: good}, nil)
	require.NoError(t, err)
	bad := deploymentTestArchive(t, "exit(42);")
	result, err := e.rolloutArchive(t.Context(), Deployment{Reference: "bad-release"}, bad, io.Discard)
	require.Error(t, err)
	assert.Empty(t, result.Reference)
	assert.Equal(t, "releases/shop", currentRelease(t, e))
	_, err = e.rolloutArchive(t.Context(), Deployment{Reference: "bad-release"}, bad, io.Discard)
	require.Error(t, err, "failed preparation must not be reused")
	// The process-owned lock is released after a failed preparation.
	_, err = e.RolloutDeployment(t.Context(), Deployment{Reference: good}, nil)
	require.NoError(t, err)
}

func TestSSHRolloutRejectsUnsafeInputsBeforeSSH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	archive := deploymentTestArchive(t, "")
	for _, dir := range []string{"relative/current", "/var/www/shop", "/current"} {
		e := &SSH{directory: dir}
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
		require.ErrorContains(t, err, "ssh.directory")
	}
	e := &SSH{directory: "/var/www/shop/current"}
	for _, reference := range []string{"", "missing", t.TempDir()} {
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: reference}, nil)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "executable file not found", "must fail before looking for ssh")
	}
	testhelper.WriteFile(t, archive, "not an archive")
	_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
	require.ErrorContains(t, err, "validate deployment archive")
}

func TestSSHRolloutRefusesRealCurrentAndMissingConfiguration(t *testing.T) {
	e := localDeploymentSSH(t)
	// Runtime configuration is validated by the helper, not by assuming one
	// particular dotenv file must be shared.
	archive := deploymentTestArchive(t, `if (!is_file('.env.local') && !getenv('DATABASE_URL')) { exit(9); }`)
	testhelper.WriteFile(t, filepath.Join(e.directory, "keep"), "existing application")
	_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(e.directory, "keep"))
	require.NoError(t, os.RemoveAll(e.directory))
	require.NoError(t, os.Remove(filepath.Join(filepath.Dir(e.directory), "shared/.env.local")))
	_, err = e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
	require.Error(t, err)
	assert.NoFileExists(t, e.directory)
}

func TestSSHRolloutUsesConfiguredSharedPaths(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	files := []string{"custom/runtime.ini"}
	directories := []string{"custom/persistent-data"}
	e.env = &shop.EnvironmentConfig{SSH: &shop.EnvironmentSSHConfig{
		Shared: &shop.EnvironmentSSHSharedConfig{Files: &files, Directories: &directories},
	}}
	testhelper.WriteFile(t, filepath.Join(root, "shared/custom/runtime.ini"), "server configuration")
	testhelper.WriteFile(t, filepath.Join(root, "shared/custom/persistent-data/upload"), "persistent custom data")
	archive := deploymentTestArchive(t, `
if (!is_link('custom/runtime.ini') || !is_link('custom/persistent-data')) { exit(9); }
if (is_link('.env.local') || is_link('config/jwt') || is_link('public/media')) { exit(9); }
`)
	for range 2 {
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
		require.NoError(t, err)
		data, err := os.ReadFile(filepath.Join(e.directory, "custom/persistent-data/upload"))
		require.NoError(t, err)
		assert.Equal(t, "persistent custom data", string(data))
	}
	assert.NoFileExists(t, filepath.Join(root, "shared/install.lock"))
	assert.NoDirExists(t, filepath.Join(root, "shared/config/jwt"))
}

func TestSSHRolloutCanDisableAllSharing(t *testing.T) {
	e := localDeploymentSSH(t)
	empty := []string{}
	e.env = &shop.EnvironmentConfig{SSH: &shop.EnvironmentSSHConfig{
		Shared: &shop.EnvironmentSSHSharedConfig{Files: &empty, Directories: &empty},
	}}
	archive := deploymentTestArchive(t, `
if (file_exists('.env.local') || is_link('config/jwt') || is_link('public/media')) { exit(9); }
`)
	_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(e.directory, "config/jwt/test.key"))
	assert.NoFileExists(t, filepath.Join(filepath.Dir(e.directory), "shared/install.lock"))
}

func TestSSHRolloutRejectsInvalidSharedPathsBeforeSSH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, name := range []string{"../outside", "public"} {
		directories := []string{name}
		e := &SSH{directory: "/var/www/shop/current", env: &shop.EnvironmentConfig{
			SSH: &shop.EnvironmentSSHConfig{Shared: &shop.EnvironmentSSHSharedConfig{Directories: &directories}},
		}}
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: "unused.tar.gz"}, nil)
		require.ErrorContains(t, err, "ssh.shared")
	}
}

func TestSSHRolloutWaitsForClientAuthorization(t *testing.T) {
	e := localDeploymentSSH(t)
	archive := deploymentTestArchive(t, "")
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	input := sshRolloutInput{
		Root: filepath.Dir(e.directory), Reference: "without-client-authorization", Release: "shop",
		Deployment: archive, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)),
	}
	payload, err := json.Marshal(input)
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), deploymentPHP(t), "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload))
	cmd.Stdin = io.MultiReader(bytes.NewReader(data), strings.NewReader("CONTINUE "+input.Reference+"\n")) // EOF instead of ACTIVATE.
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(output), "READY "+input.Reference)
	assert.Contains(t, string(output), "Client did not authorize activation")
	assert.NoFileExists(t, e.directory)
}

func TestSSHRolloutRejectsDamagedUploads(t *testing.T) {
	e := localDeploymentSSH(t)
	archive := deploymentTestArchive(t, "")
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	for _, tc := range []struct {
		name, checksum, message string
		size                    int64
	}{
		{"checksum", strings.Repeat("0", 64), "Archive checksum mismatch", int64(len(data))},
		{"truncated", hex.EncodeToString(sum[:]), "Incomplete archive upload", int64(len(data)) + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := sshRolloutInput{
				Root: filepath.Dir(e.directory), Reference: tc.name, Release: tc.name,
				Deployment: archive, SHA256: tc.checksum, Size: tc.size,
			}
			payload, err := json.Marshal(input)
			require.NoError(t, err)
			cmd := exec.CommandContext(t.Context(), deploymentPHP(t), "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload))
			cmd.Stdin = bytes.NewReader(data)
			output, err := cmd.CombinedOutput()
			require.Error(t, err)
			assert.Contains(t, string(output), tc.message)
			assert.NoFileExists(t, e.directory)
			assert.NoDirExists(t, filepath.Join(input.Root, "releases", input.Reference))
			artifacts, globErr := filepath.Glob(filepath.Join(input.Root, ".shopware-cli/artifacts/*"))
			require.NoError(t, globErr)
			assert.Empty(t, artifacts)
		})
	}
}

func TestSSHRolloutSerializesDeployments(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	archive := deploymentTestArchive(t, `
$shared = dirname(getcwd(), 2) . '/shared';
file_put_contents("$shared/waiting", '1');
$deadline = microtime(true) + 15;
while (!file_exists("$shared/continue")) {
    if (microtime(true) > $deadline) { exit(1); }
    usleep(10000);
}`)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	defer func() { _ = os.WriteFile(filepath.Join(root, "shared/continue"), []byte("1"), 0o600) }()
	done := make(chan error, 1)
	go func() {
		_, err := e.RolloutDeployment(ctx, Deployment{Reference: archive}, nil)
		done <- err
	}()
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(root, "shared/waiting"))
		return err == nil
	}, 10*time.Second, 10*time.Millisecond)
	err := e.applyDeploymentInitialization(ctx, sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_URL": "https://shop.example.com"},
	})
	require.ErrorContains(t, err, "holds the lock", "initialization must not change configuration during a rollout")
	good := deploymentTestArchive(t, "")
	_, err = e.RolloutDeployment(ctx, Deployment{Reference: good}, nil)
	require.Error(t, err, "a second client must not deploy while the first holds the lock")
	assert.NoFileExists(t, e.directory)
	testhelper.WriteFile(t, filepath.Join(root, "shared/continue"), "1")
	require.NoError(t, <-done)
}

func TestDeploymentHandshakeDoesNotAuthorizeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for _, response := range []string{"CACHED id\nPREPARE id\nREADY id\n", "REUSE id\nREADY id\n"} {
		var stdin bytes.Buffer
		_, err := exchangeSSHDeployment(
			ctx,
			&stdin,
			bufio.NewReader(strings.NewReader(response)),
			bytes.NewReader(nil),
			sshRolloutInput{Reference: "id"},
			ci.New(io.Discard),
		)
		require.ErrorIs(t, err, context.Canceled)
		assert.NotContains(t, stdin.String(), "ACTIVATE")
	}
	require.Error(t, readRolloutMessage(bufio.NewReader(strings.NewReader(strings.Repeat("x", 5000))), "READY"))
	require.ErrorIs(t, readRolloutMessage(bufio.NewReader(strings.NewReader("")), "READY"), io.EOF)
}

func TestDeploymentHandshakeSkipsCachedArchive(t *testing.T) {
	for _, tc := range []struct {
		name, environment, headingPrefix, headingSuffix, endMarker string
		ansi                                                       bool
	}{
		{"terminal", "", "--- ", " ---", " finished in ", false},
		{"GitHub", "GITHUB_ACTIONS", "::group::", "", "::endgroup::", false},
		{"GitLab", "GITLAB_CI", "\r\x1b[0K", "", "section_end:", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GITHUB_ACTIONS", "")
			t.Setenv("GITLAB_CI", "")
			if tc.environment != "" {
				t.Setenv(tc.environment, "true")
			}
			archive := bytes.NewBufferString("must remain unread")
			var stdin, output bytes.Buffer
			_, err := exchangeSSHDeployment(
				t.Context(),
				&stdin,
				bufio.NewReader(strings.NewReader("CACHED id\nPREPARE id\nREADY id\nACTIVE id\n")),
				archive,
				sshRolloutInput{Reference: "id", Size: int64(archive.Len())},
				ci.New(&output),
			)
			require.NoError(t, err)
			assert.Equal(t, "must remain unread", archive.String())
			assert.Equal(t, "CONTINUE id\nACTIVATE id\n", stdin.String())
			for _, heading := range []string{"Using cached deployment artifact", "Preparing release", "Activating release"} {
				assert.Contains(t, output.String(), tc.headingPrefix+heading+tc.headingSuffix)
			}
			assert.Equal(t, 3, strings.Count(output.String(), tc.endMarker))
			assert.Equal(t, tc.ansi, strings.Contains(output.String(), "\x1b["))
		})
	}
}

func TestDeploymentHandshakeUploadsMissingArchive(t *testing.T) {
	archive := "archive bytes"
	var stdin bytes.Buffer
	_, err := exchangeSSHDeployment(
		t.Context(),
		&stdin,
		bufio.NewReader(strings.NewReader("UPLOAD id\nPREPARE id\nREADY id\nACTIVE id\n")),
		strings.NewReader(archive),
		sshRolloutInput{Reference: "id", Size: int64(len(archive))},
		ci.New(io.Discard),
	)
	require.NoError(t, err)
	assert.Equal(t, archive+"CONTINUE id\nACTIVATE id\n", stdin.String())
	_, err = readRolloutAction(bufio.NewReader(strings.NewReader("UNKNOWN id\n")), "id")
	require.ErrorContains(t, err, "unexpected SSH deployment response")
}

func TestDeploymentHandshakeReusesPreparedRelease(t *testing.T) {
	for _, tc := range []struct {
		name, response, wantInput, reference string
		unchanged                            bool
	}{
		{"inactive", "REUSE id\nREADY id\nACTIVE id\n", "ACTIVATE id\n", "id", false},
		{"active", "UNCHANGED id\nprevious-id\n", "", "previous-id", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			archive := bytes.NewBufferString("must remain unread")
			var stdin, output bytes.Buffer
			result, err := exchangeSSHDeployment(t.Context(), &stdin,
				bufio.NewReader(strings.NewReader(tc.response)), archive,
				sshRolloutInput{Reference: "id", Deployment: "happy-euclid"}, ci.New(&output))
			require.NoError(t, err)
			assert.Equal(t, tc.wantInput, stdin.String())
			assert.Equal(t, "must remain unread", archive.String())
			assert.Equal(t, tc.reference, result.Reference)
			assert.Equal(t, tc.unchanged, result.Unchanged)
			assert.True(t, result.Active)
			assert.Equal(t, "happy-euclid", result.Deployment.Reference)
			assert.NotContains(t, output.String(), "Preparing release")
			if tc.unchanged {
				assert.Empty(t, output.String())
			}
		})
	}
}

func TestSSHNamedReleaseRejectsDifferentArchive(t *testing.T) {
	e := localDeploymentSSH(t)
	archive := deploymentTestArchive(t, "")
	first, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
	require.NoError(t, err)
	different := deploymentTestArchive(t, "echo 'different build';")
	var logs bytes.Buffer
	_, err = e.RolloutDeployment(t.Context(), Deployment{Reference: different}, &logs)
	require.Error(t, err)
	assert.Contains(t, logs.String(), "checksum")
	assert.NotContains(t, logs.String(), "Uploading deployment artifact")
	assert.Equal(t, "releases/shop", currentRelease(t, e))
	rollouts, err := e.ListRollouts(t.Context())
	require.NoError(t, err)
	require.Len(t, rollouts, 1)
	assert.Equal(t, first.Reference, rollouts[0].Reference)
	_, err = e.rolloutArchive(t.Context(), Deployment{Reference: "another-release"}, archive, io.Discard)
	require.NoError(t, err)
	logs.Reset()
	_, err = e.RolloutDeployment(t.Context(), Deployment{Reference: different}, &logs)
	require.Error(t, err)
	assert.Contains(t, logs.String(), "checksum")
	assert.Equal(t, "releases/another-release", currentRelease(t, e))
}

func TestSSHNamedReleaseRejectsUnsafeNames(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	e := &SSH{directory: "/var/www/shop/current"}
	for _, reference := range []string{".tar.gz", "..", ".hidden.tar.gz", "name with spaces.tar.gz", strings.Repeat("x", 129)} {
		_, err := e.rolloutArchive(t.Context(), Deployment{Reference: reference}, "missing.tar.gz", io.Discard)
		require.ErrorContains(t, err, "invalid deployment name")
	}
}

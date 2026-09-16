package executor

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

func localDeploymentSSH(t *testing.T) *SSHExecutor {
	t.Helper()
	php := deploymentPHP(t)
	t.Setenv("DATABASE_URL", "")
	bin := t.TempDir()
	// Execute the SSH remote command locally. No network or sshd is involved.
	testhelper.WriteFile(t, filepath.Join(bin, "ssh"), "#!/bin/sh\nfor arg do command=\"$arg\"; done\nexec /bin/sh -c \"$command\"\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "ssh"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	// Spaces and apostrophes must survive SSH's remote shell.
	root = filepath.Join(root, "shop's releases")
	testhelper.WriteFile(t, filepath.Join(root, "shared/.env.local"), "DATABASE_URL=provided-on-server\n")
	testhelper.WriteFile(t, filepath.Join(root, "shared/public/media/upload"), "persistent")
	return &SSHExecutor{host: "example.invalid", directory: filepath.Join(root, "current"), phpBinary: php}
}

func deploymentTestArchive(t *testing.T, helper, console string) string {
	t.Helper()
	root := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(root, "composer.json"), "{}")
	testhelper.WriteFile(t, filepath.Join(root, "vendor/bin/shopware-deployment-helper"), "<?php\n"+helper)
	testhelper.WriteFile(t, filepath.Join(root, "bin/console"), "<?php\n"+console)
	testhelper.WriteFile(t, filepath.Join(root, "public/bundles/app.js"), "built")
	testhelper.WriteFile(t, filepath.Join(root, "config/jwt/test.key"), "fixture-key")
	file := filepath.Join(t.TempDir(), "shop.tar.gz")
	out, err := os.Create(file)
	require.NoError(t, err)
	require.NoError(t, archiver.WriteTarGz(t.Context(), out, root, nil))
	require.NoError(t, out.Close())
	return file
}

func currentRelease(t *testing.T, e *SSHExecutor) string {
	t.Helper()
	target, err := os.Readlink(e.directory)
	require.NoError(t, err)
	return target
}

func TestSSHRolloutPreparesAndActivatesReleases(t *testing.T) {
	e := localDeploymentSSH(t)
	archive := deploymentTestArchive(t,
		`if (!is_link('.env.local') || !is_link('public/media')) { exit(9); }
if (file_get_contents('.env') !== "APP_ENV=prod\nAPP_DEBUG=0\n") { exit(9); }
file_put_contents('helper-ran', implode(' ', array_slice($argv, 1)));
fwrite(STDOUT, "helper log\n");`,
		`if (!is_file('helper-ran') || !is_link('install.lock')) { exit(9); }
file_put_contents('checks-ran', implode(' ', array_slice($argv, 1)));`)
	deployment := Deployment{Reference: archive}
	first, err := e.RolloutDeployment(t.Context(), deployment)
	require.NoError(t, err)
	assert.True(t, first.Active)
	assert.Equal(t, deployment, first.Deployment)
	assert.Equal(t, "releases/"+first.Reference, currentRelease(t, e))
	root := filepath.Dir(e.directory)
	assert.FileExists(t, filepath.Join(e.directory, "helper-ran"))
	assert.FileExists(t, filepath.Join(e.directory, "checks-ran"))
	assert.FileExists(t, filepath.Join(root, "shared/config/jwt/test.key"))
	assert.FileExists(t, filepath.Join(root, "shared/install.lock"))
	assert.NoDirExists(t, filepath.Join(root, "shared/var/cache"))

	second, err := e.RolloutDeployment(t.Context(), deployment)
	require.NoError(t, err)
	assert.NotEqual(t, first.Reference, second.Reference, "a repeated deployment is a new rollout")
	assert.Equal(t, "releases/"+second.Reference, currentRelease(t, e))
	assert.DirExists(t, filepath.Join(root, "releases", first.Reference))
	data, err := os.ReadFile(filepath.Join(e.directory, "public/media/upload"))
	require.NoError(t, err)
	assert.Equal(t, "persistent", string(data))
	metadata, err := os.ReadFile(filepath.Join(root, ".shopware-cli/rollouts", second.Reference+".json"))
	require.NoError(t, err)
	var record map[string]string
	require.NoError(t, json.Unmarshal(metadata, &record))
	assert.Equal(t, "successful", record["status"])
	assert.Equal(t, archive, record["deployment"])
	uploads, err := filepath.Glob(filepath.Join(root, ".shopware-cli/*.tar.gz"))
	require.NoError(t, err)
	assert.Empty(t, uploads)
	assert.FileExists(t, archive, "rollout must retain the local deployment archive")
}

func TestSSHRolloutFailuresKeepCurrent(t *testing.T) {
	e := localDeploymentSSH(t)
	good := deploymentTestArchive(t, "", "")
	first, err := e.RolloutDeployment(t.Context(), Deployment{Reference: good})
	require.NoError(t, err)
	for _, tc := range []struct{ name, helper, console string }{
		{"helper failure", "exit(42);", ""},
		{"check failure", "", "exit(43);"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := deploymentTestArchive(t, tc.helper, tc.console)
			result, err := e.RolloutDeployment(t.Context(), Deployment{Reference: bad})
			require.Error(t, err)
			assert.Empty(t, result.Reference)
			assert.Equal(t, "releases/"+first.Reference, currentRelease(t, e))
		})
	}
	// The process-owned lock is released after a failed preparation.
	_, err = e.RolloutDeployment(t.Context(), Deployment{Reference: good})
	require.NoError(t, err)
}

func TestSSHRolloutRejectsUnsafeInputsBeforeSSH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	archive := deploymentTestArchive(t, "", "")
	for _, dir := range []string{"relative/current", "/var/www/shop", "/current"} {
		e := &SSHExecutor{directory: dir}
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive})
		require.ErrorContains(t, err, "ssh.directory")
	}
	e := &SSHExecutor{directory: "/var/www/shop/current"}
	for _, reference := range []string{"", "missing", t.TempDir()} {
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: reference})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "executable file not found", "must fail before looking for ssh")
	}
	testhelper.WriteFile(t, archive, "not an archive")
	_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive})
	require.ErrorContains(t, err, "validate deployment archive")
}

func TestSSHRolloutRefusesRealCurrentAndMissingConfiguration(t *testing.T) {
	e := localDeploymentSSH(t)
	// Runtime configuration is validated by the helper, not by assuming one
	// particular dotenv file must be shared.
	archive := deploymentTestArchive(t, `if (!is_file('.env.local') && !getenv('DATABASE_URL')) { exit(9); }`, "")
	testhelper.WriteFile(t, filepath.Join(e.directory, "keep"), "existing application")
	_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive})
	require.Error(t, err)
	assert.FileExists(t, filepath.Join(e.directory, "keep"))
	require.NoError(t, os.RemoveAll(e.directory))
	require.NoError(t, os.Remove(filepath.Join(filepath.Dir(e.directory), "shared/.env.local")))
	_, err = e.RolloutDeployment(t.Context(), Deployment{Reference: archive})
	require.Error(t, err)
	assert.NoFileExists(t, e.directory)
}

func TestSSHRolloutUsesConfiguredSharedPaths(t *testing.T) {
	e := localDeploymentSSH(t)
	root := filepath.Dir(e.directory)
	files := []string{"custom/runtime.ini"}
	directories := []string{"custom/persistent-data"}
	e.envCfg = &shop.EnvironmentConfig{SSH: &shop.EnvironmentSSHConfig{
		Shared: &shop.EnvironmentSSHSharedConfig{Files: &files, Directories: &directories},
	}}
	testhelper.WriteFile(t, filepath.Join(root, "shared/custom/runtime.ini"), "server configuration")
	testhelper.WriteFile(t, filepath.Join(root, "shared/custom/persistent-data/upload"), "persistent custom data")
	archive := deploymentTestArchive(t, `
if (!is_link('custom/runtime.ini') || !is_link('custom/persistent-data')) { exit(9); }
if (is_link('.env.local') || is_link('config/jwt') || is_link('public/media')) { exit(9); }
`, `if (!is_file('install.lock') || is_link('install.lock')) { exit(9); }`)
	for range 2 {
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive})
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
	e.envCfg = &shop.EnvironmentConfig{SSH: &shop.EnvironmentSSHConfig{
		Shared: &shop.EnvironmentSSHSharedConfig{Files: &empty, Directories: &empty},
	}}
	archive := deploymentTestArchive(t, `
if (file_exists('.env.local') || is_link('config/jwt') || is_link('public/media')) { exit(9); }
`, `if (!is_file('install.lock') || is_link('install.lock')) { exit(9); }`)
	_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: archive})
	require.NoError(t, err)
	assert.FileExists(t, filepath.Join(e.directory, "config/jwt/test.key"))
	assert.NoFileExists(t, filepath.Join(filepath.Dir(e.directory), "shared/install.lock"))
}

func TestSSHRolloutRejectsInvalidSharedPathsBeforeSSH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, name := range []string{"../outside", "public"} {
		directories := []string{name}
		e := &SSHExecutor{directory: "/var/www/shop/current", envCfg: &shop.EnvironmentConfig{
			SSH: &shop.EnvironmentSSHConfig{Shared: &shop.EnvironmentSSHSharedConfig{Directories: &directories}},
		}}
		_, err := e.RolloutDeployment(t.Context(), Deployment{Reference: "unused.tar.gz"})
		require.ErrorContains(t, err, "ssh.shared")
	}
}

func TestSSHRolloutWaitsForClientAuthorization(t *testing.T) {
	e := localDeploymentSSH(t)
	archive := deploymentTestArchive(t, "", "")
	data, err := os.ReadFile(archive)
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	input := sshRolloutInput{
		Root: filepath.Dir(e.directory), Reference: "without-client-authorization",
		Deployment: archive, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(data)), PHP: e.php(),
	}
	payload, err := json.Marshal(input)
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), e.php(), "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload))
	cmd.Stdin = bytes.NewReader(data) // EOF instead of ACTIVATE.
	output, err := cmd.CombinedOutput()
	require.Error(t, err)
	assert.Contains(t, string(output), "READY "+input.Reference)
	assert.Contains(t, string(output), "Client did not authorize activation")
	assert.NoFileExists(t, e.directory)
}

func TestSSHRolloutRejectsDamagedUploads(t *testing.T) {
	e := localDeploymentSSH(t)
	archive := deploymentTestArchive(t, "", "")
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
				Root: filepath.Dir(e.directory), Reference: tc.name,
				Deployment: archive, SHA256: tc.checksum, Size: tc.size, PHP: e.php(),
			}
			payload, err := json.Marshal(input)
			require.NoError(t, err)
			cmd := exec.CommandContext(t.Context(), e.php(), "-r", strings.TrimPrefix(sshDeploymentScript, "<?php\n"), "--", string(payload))
			cmd.Stdin = bytes.NewReader(data)
			output, err := cmd.CombinedOutput()
			require.Error(t, err)
			assert.Contains(t, string(output), tc.message)
			assert.NoFileExists(t, e.directory)
			assert.NoDirExists(t, filepath.Join(input.Root, "releases", input.Reference))
			assert.NoFileExists(t, filepath.Join(input.Root, ".shopware-cli", input.Reference+".tar.gz"))
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
}`, "")
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	defer func() { _ = os.WriteFile(filepath.Join(root, "shared/continue"), []byte("1"), 0o600) }()
	done := make(chan error, 1)
	go func() {
		_, err := e.RolloutDeployment(ctx, Deployment{Reference: archive})
		done <- err
	}()
	require.Eventually(t, func() bool {
		_, err := os.Stat(filepath.Join(root, "shared/waiting"))
		return err == nil
	}, 10*time.Second, 10*time.Millisecond)
	good := deploymentTestArchive(t, "", "")
	_, err := e.RolloutDeployment(ctx, Deployment{Reference: good})
	require.Error(t, err, "a second client must not deploy while the first holds the lock")
	assert.NoFileExists(t, e.directory)
	testhelper.WriteFile(t, filepath.Join(root, "shared/continue"), "1")
	require.NoError(t, <-done)
}

func TestDeploymentHandshakeDoesNotAuthorizeCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdin bytes.Buffer
	err := exchangeSSHDeployment(ctx, &stdin, bufio.NewReader(strings.NewReader("READY id\n")), bytes.NewReader(nil), sshRolloutInput{Reference: "id"})
	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, stdin.String(), "ACTIVATE")
	require.Error(t, readRolloutMessage(bufio.NewReader(strings.NewReader(strings.Repeat("x", 5000))), "READY"))
	require.ErrorIs(t, readRolloutMessage(bufio.NewReader(strings.NewReader("")), "READY"), io.EOF)
}

package ssh

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestCachetoolBackpressureStillCleansWebEndpoint(t *testing.T) {
	php := deploymentPHP(t)
	root := t.TempDir()
	phar := filepath.Join(root, "verified-fixture")
	testhelper.WriteFile(t, phar, "fixture")
	sum := sha256.Sum256([]byte("fixture"))
	web := filepath.Join(root, "endpoint")
	state, err := json.Marshal(map[string]any{
		"directory": root, "timeout": 1, "web_directory": web,
		"phar": phar, "sha256": hex.EncodeToString(sum[:]),
		"command": []string{php, "-r", `echo str_repeat("x", 1048576); sleep(10);`},
	})
	require.NoError(t, err)
	script := strings.TrimPrefix(sshCachetoolScript, "<?php\n") + `
$state = json_decode($argv[1], true, 512, JSON_THROW_ON_ERROR);
$failed = false;
try { deploymentCachetoolReset($state); }
catch (Throwable $error) { $failed = true; deploymentReportReset($error->getMessage()); }
echo json_encode(['failed' => $failed, 'cleaned' => !file_exists($state['web_directory'])]);
`
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, php, "-r", script, "--", string(state))
	cmd.WaitDelay = time.Second
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)
	defer func() { _ = stderr.Close() }()
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	start := time.Now()
	// Intentionally leave stderr unread so forwarding cannot block cleanup.
	require.NoError(t, cmd.Run())
	assert.Less(t, time.Since(start), 4*time.Second)
	assert.JSONEq(t, `{"failed":true,"cleaned":true}`, stdout.String())
	assert.NoDirExists(t, web)
}

func TestBoundedCommandCapturesExpectedNonzeroExit(t *testing.T) {
	php := deploymentPHP(t)
	script := strings.TrimPrefix(sshCachetoolScript, "<?php\n") + `
echo deploymentRunBoundedCommand([PHP_BINARY, '-r', 'echo "captured"; exit(1);'], '.', 2, true, [0, 1]);
`
	cmd := exec.CommandContext(t.Context(), php, "-r", script)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Equal(t, "captured", string(output))
}

func TestBoundedCommandLimitsCapturedOutput(t *testing.T) {
	php := deploymentPHP(t)
	script := strings.TrimPrefix(sshCachetoolScript, "<?php\n") + `
try { deploymentRunBoundedCommand([PHP_BINARY, '-r', 'echo str_repeat("x", 2097152);'], '.', 2, true); }
catch (Throwable $error) { echo $error->getMessage(); }
`
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, php, "-r", script).CombinedOutput()
	require.NoError(t, err, string(output))
	assert.Contains(t, string(output), "output exceeds 1 MiB")
}

func TestCachetoolDisabledIncludeReachesBackend(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	testhelper.WriteFile(t, ".shopware-project.yml", "include: [override.yml]\nenvironments:\n  production:\n    type: ssh\n    ssh:\n      host: example.invalid\n      directory: /srv/shop/current\n      cachetool: {enabled: true, adapter: fcgi}\n")
	testhelper.WriteFile(t, "override.yml", "environments:\n  production:\n    ssh:\n      cachetool: {enabled: false}\n")
	cfg, err := shop.ReadConfig(t.Context(), ".shopware-project.yml", false)
	require.NoError(t, err)
	backend, err := New(root, ".shopware-project.yml", cfg, cfg.Environments["production"])
	require.NoError(t, err)
	settings, err := backend.(*SSH).cachetoolInput()
	require.NoError(t, err)
	assert.Nil(t, settings)
}

func TestCachetoolExternalPathResolvesSymlinkAncestors(t *testing.T) {
	php := deploymentPHP(t)
	root := t.TempDir()
	external := t.TempDir()
	release := filepath.Join(root, "releases", "first")
	require.NoError(t, os.MkdirAll(release, 0o755))
	require.NoError(t, os.Symlink(external, filepath.Join(release, "public")))
	require.NoError(t, os.Symlink(release, filepath.Join(root, "current")))
	require.NoError(t, os.Symlink(release, filepath.Join(root, "live")))
	alias := filepath.Join(t.TempDir(), "release-alias")
	require.NoError(t, os.Symlink(release, alias))
	safeAlias := filepath.Join(t.TempDir(), "external-alias")
	require.NoError(t, os.Symlink(external, safeAlias))
	indirect := filepath.Join(t.TempDir(), "indirect")
	require.NoError(t, os.Symlink(root+"/current/public", indirect))
	customAlias := filepath.Join(t.TempDir(), "custom-alias")
	require.NoError(t, os.Symlink(root+"/live/public", customAlias))
	for _, tc := range []struct {
		name, path string
		valid      bool
	}{
		{"current public external symlink", root + "/current/public", false},
		{"release public external symlink", release + "/public", false},
		{"release alias external child", alias + "/public", false},
		{"alias through current to external public", indirect, false},
		{"existing release alias nonexistent child", alias + "/new/tmp", false},
		{"safe external alias", safeAlias + "/./new", true},
		{"similar external name", root + "/releases-other/new", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			script := strings.TrimPrefix(sshCachetoolScript, "<?php\n") + `
try { echo deploymentCachetoolExternalPath($argv[1], $argv[2], 'web_path'); }
catch (Throwable $error) { fwrite(STDERR, $error->getMessage()); exit(1); }
`
			output, err := exec.CommandContext(t.Context(), php, "-r", script, "--", tc.path, root).CombinedOutput()
			if tc.valid {
				require.NoError(t, err, string(output))
			} else {
				require.Error(t, err)
				assert.Contains(t, string(output), "inside releases or the active symlink")
			}
		})
	}
	for _, location := range []string{root + "/live/public", customAlias} {
		script := strings.TrimPrefix(sshCachetoolScript, "<?php\n") + `
try { echo deploymentCachetoolExternalPath($argv[1], $argv[2], 'web_path', 'live'); }
catch (Throwable $error) { exit(1); }
`
		_, err := exec.CommandContext(t.Context(), php, "-r", script, "--", location, root).Output()
		require.Error(t, err)
	}
}

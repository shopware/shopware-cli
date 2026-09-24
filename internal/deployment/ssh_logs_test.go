package deployment

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSSHDeploymentHelperLogs(t *testing.T) {
	for _, exit := range []string{"0", "42"} {
		t.Run("exit "+exit, func(t *testing.T) {
			e := localDeploymentSSH(t)
			archive := deploymentTestArchive(t, `
fwrite(STDOUT, str_repeat("stdout\n", 10000));
fwrite(STDERR, "stderr without trailing newline");
exit(`+exit+`);`)
			expected := strings.Repeat("stdout\n", 10000) + "stderr without trailing newline"
			var live bytes.Buffer
			_, err := e.rolloutArchive(t.Context(), Deployment{Reference: "happy-euclid"}, archive, &live)
			if exit == "0" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.NoFileExists(t, e.directory)
			}
			assert.Contains(t, live.String(), expected, "both streams must still reach the client")
			logPath := filepath.Join(filepath.Dir(e.directory), ".shopware-cli/logs/happy-euclid.log")
			data, err := os.ReadFile(logPath)
			require.NoError(t, err)
			assert.Equal(t, expected, string(data), "store only helper output, not sections or protocol")
			info, err := os.Stat(logPath)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
			info, err = os.Stat(filepath.Dir(logPath))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o700), info.Mode().Perm())
			require.NoError(t, os.Remove(archive))

			for _, reference := range []string{"happy-euclid", "/old/checkout/happy-euclid.tar.gz"} {
				var stored bytes.Buffer
				require.NoError(t, e.WriteDeploymentLogs(t.Context(), Deployment{Reference: reference}, &stored))
				assert.Equal(t, expected, stored.String())
			}
		})
	}
}

func TestSSHDeploymentLogsMissing(t *testing.T) {
	e := localDeploymentSSH(t)
	var output bytes.Buffer
	err := e.WriteDeploymentLogs(t.Context(), Deployment{Reference: "missing"}, &output)
	require.ErrorContains(t, err, "No deployment helper log")
	assert.Empty(t, output.String())
	assert.NoDirExists(t, filepath.Join(filepath.Dir(e.directory), ".shopware-cli"), "reading logs must not initialize deployment storage")
}

func TestSSHDeploymentLogsRefuseSymlinks(t *testing.T) {
	for _, scenario := range []string{"log", "directory"} {
		t.Run(scenario, func(t *testing.T) {
			e := localDeploymentSSH(t)
			storage := filepath.Join(filepath.Dir(e.directory), ".shopware-cli")
			require.NoError(t, os.MkdirAll(storage, 0o700))
			outside := t.TempDir()
			testhelper.WriteFile(t, filepath.Join(outside, "happy-euclid.log"), "must not disclose")
			if scenario == "directory" {
				require.NoError(t, os.Symlink(outside, filepath.Join(storage, "logs")))
			} else {
				require.NoError(t, os.Mkdir(filepath.Join(storage, "logs"), 0o700))
				require.NoError(t, os.Symlink(filepath.Join(outside, "happy-euclid.log"), filepath.Join(storage, "logs/happy-euclid.log")))
			}
			var output bytes.Buffer
			require.Error(t, e.WriteDeploymentLogs(t.Context(), Deployment{Reference: "happy-euclid"}, &output))
			assert.Empty(t, output.String())
		})
	}
}

func TestSSHDeploymentHelperLogsNeverOverwrite(t *testing.T) {
	e := localDeploymentSSH(t)
	logPath := filepath.Join(filepath.Dir(e.directory), ".shopware-cli/logs/happy-euclid.log")
	testhelper.WriteFile(t, logPath, "previous helper output")
	archive := deploymentTestArchive(t, `file_put_contents("helper-ran", "1");`)
	_, err := e.rolloutArchive(t.Context(), Deployment{Reference: "happy-euclid"}, archive, io.Discard)
	require.Error(t, err)
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Equal(t, "previous helper output", string(data))
	assert.NoFileExists(t, filepath.Join(filepath.Dir(e.directory), "releases/happy-euclid/helper-ran"))
}

func TestSSHDeploymentLogsCancellationAndValidation(t *testing.T) {
	e := localDeploymentSSH(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, e.WriteDeploymentLogs(ctx, Deployment{Reference: "happy-euclid"}, io.Discard), context.Canceled)
	t.Setenv("PATH", t.TempDir())
	for _, reference := range []string{"", "..", ".hidden", "name\ninjection"} {
		require.ErrorContains(t, e.WriteDeploymentLogs(t.Context(), Deployment{Reference: reference}, io.Discard), "invalid deployment name")
	}
}

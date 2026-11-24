package ssh

import (
	"bytes"
	"encoding/json"
	"io"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

// localDeploymentCommand prepares a packaged command for the local PHP binary
// in place of a remote host. The returned archive must start the command's stdin.
func localDeploymentCommand(t *testing.T, command string, options any) (*exec.Cmd, io.Reader) {
	t.Helper()
	php, err := exec.LookPath("php")
	if err != nil {
		t.Skip("PHP required")
	}
	payload, err := deploymentPayload(command)
	require.NoError(t, err)
	var args []string
	if options != nil {
		data, err := json.Marshal(options)
		require.NoError(t, err)
		args = []string{string(data)}
	}
	return exec.CommandContext(t.Context(), php, payload.PHPArgs(args...)...), payload.Stdin()
}

// runLocalDeploymentCommand returns the stdout of a packaged command that must
// succeed without writing to stderr.
func runLocalDeploymentCommand(t *testing.T, command string, options any) []byte {
	t.Helper()
	cmd, archive := localDeploymentCommand(t, command, options)
	cmd.Stdin = archive
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	require.NoError(t, err, "%s", stderr.String())
	require.Empty(t, stderr.String())
	return output
}

// runLocalDeploymentInit sends its options on stdin, like secret initialization values.
func runLocalDeploymentInit(t *testing.T, input sshDeploymentInitInput) []byte {
	t.Helper()
	data, err := json.Marshal(input)
	require.NoError(t, err)
	cmd, archive := localDeploymentCommand(t, "init", nil)
	cmd.Stdin = io.MultiReader(archive, bytes.NewReader(data))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	require.NoError(t, err, "%s", stderr.String())
	return output
}

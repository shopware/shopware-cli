package verifier

import (
	"os/exec"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// testExitError runs a shell command to get a real process exit error.
func testExitError(t *testing.T, code int) error {
	t.Helper()

	script := "exit " + strconv.Itoa(code)
	cmd := exec.CommandContext(t.Context(), "sh", "-c", script)
	err := cmd.Run()

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	return err
}

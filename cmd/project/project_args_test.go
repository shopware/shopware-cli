package project

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// format, validate and fix take at most one optional path, so extra paths
// must be rejected instead of silently ignored.
func TestOptionalPathCommandsRejectExtraArgs(t *testing.T) {
	commands := []string{"format", "validate", "fix"}

	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			out := new(bytes.Buffer)
			projectRootCmd.SetOut(out)
			projectRootCmd.SetErr(out)
			projectRootCmd.SetArgs([]string{command, t.TempDir(), t.TempDir()})
			t.Cleanup(func() {
				projectRootCmd.SetArgs(nil)
				projectRootCmd.SetOut(nil)
				projectRootCmd.SetErr(nil)
			})

			err := projectRootCmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "accepts at most 1 arg(s), received 2")
		})
	}
}

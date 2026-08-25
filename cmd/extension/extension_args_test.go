package extension

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The single-path commands process exactly one extension, so extra paths must
// be rejected instead of silently ignored.
func TestSinglePathCommandsRejectExtraArgs(t *testing.T) {
	commands := []string{"fix", "validate", "get-name", "get-version", "get-changelog"}

	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			out := new(bytes.Buffer)
			extensionRootCmd.SetOut(out)
			extensionRootCmd.SetErr(out)
			extensionRootCmd.SetArgs([]string{command, t.TempDir(), t.TempDir()})
			t.Cleanup(func() {
				extensionRootCmd.SetArgs(nil)
				extensionRootCmd.SetOut(nil)
				extensionRootCmd.SetErr(nil)
			})

			err := extensionRootCmd.Execute()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "accepts 1 arg(s), received 2")
		})
	}
}

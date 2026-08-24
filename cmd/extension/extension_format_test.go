package extension

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionFormatRequiresPathArg(t *testing.T) {
	out := new(bytes.Buffer)
	extensionRootCmd.SetOut(out)
	extensionRootCmd.SetErr(out)
	extensionRootCmd.SetArgs([]string{"format"})
	t.Cleanup(func() {
		extensionRootCmd.SetArgs(nil)
		extensionRootCmd.SetOut(nil)
		extensionRootCmd.SetErr(nil)
	})

	err := extensionRootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires at least 1 arg")
}

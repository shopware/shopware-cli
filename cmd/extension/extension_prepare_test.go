package extension

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestExtensionPrepareCommandDeprecated(t *testing.T) {
	t.Parallel()

	assert.Contains(t, extensionPrepareCmd.Deprecated, "October 2026")
	assert.Contains(t, extensionPrepareCmd.Deprecated, "extension package")
}

func TestExtensionPreparePrintsDeprecationNotice(t *testing.T) {
	out := new(bytes.Buffer)
	extensionPrepareCmd.SetOut(out)
	extensionPrepareCmd.SetErr(out)
	extensionRootCmd.SetArgs([]string{"prepare", "--help"})
	t.Cleanup(func() {
		extensionRootCmd.SetArgs(nil)
		extensionPrepareCmd.SetOut(nil)
		extensionPrepareCmd.SetErr(nil)
	})

	require.NoError(t, extensionRootCmd.Execute())

	output := out.String()
	assert.Contains(t, output, "deprecated")
	assert.Contains(t, output, "October 2026")
}

func TestExtensionPrepareStillExecutes(t *testing.T) {
	dir := testhelper.NewApp(t, "MyExampleApp")

	out := new(bytes.Buffer)
	extensionPrepareCmd.SetOut(out)
	extensionPrepareCmd.SetErr(out)
	extensionPrepareCmd.SetContext(t.Context())
	extensionRootCmd.SetArgs([]string{"prepare", dir})
	t.Cleanup(func() {
		extensionRootCmd.SetArgs(nil)
		extensionPrepareCmd.SetOut(nil)
		extensionPrepareCmd.SetErr(nil)
	})

	require.NoError(t, extensionRootCmd.Execute())
	assert.Contains(t, out.String(), "deprecated")
	assert.Contains(t, out.String(), "October 2026")
}

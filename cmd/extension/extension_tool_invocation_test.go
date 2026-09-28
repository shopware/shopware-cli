package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/verifier"
)

func TestExtensionToolInvocationStatuses(t *testing.T) {
	assertExtensionToolInvocationStatuses(t, verifier.GetToolsOf[verifier.FixTool]())
	assertExtensionToolInvocationStatuses(t, verifier.GetToolsOf[verifier.FormatTool]())
}

func assertExtensionToolInvocationStatuses[T verifier.Tool](t *testing.T, all verifier.ToolList[T]) {
	t.Helper()
	requested, err := all.Only(all[0].Name() + "," + all[1].Name())
	require.NoError(t, err)
	selected, err := requested.Exclude(all[1].Name())
	require.NoError(t, err)

	statuses := extensionToolInvocationStatuses(all, requested, selected)
	assert.Len(t, statuses, len(all))
	assert.Equal(t, "invoked", toolStatusByName(t, statuses, all[0].Name()).Status)
	assert.Equal(t, "skipped", toolStatusByName(t, statuses, all[1].Name()).Status)
	assert.Equal(t, "excluded by --exclude", toolStatusByName(t, statuses, all[1].Name()).Reason)
	assert.Equal(t, "not selected by --only", toolStatusByName(t, statuses, all[2].Name()).Reason)

	_, err = requested.Exclude(all[2].Name())
	require.ErrorContains(t, err, "not found")
}

func TestExtensionFixAndFormatHaveExcludeFlag(t *testing.T) {
	assert.NotNil(t, extensionFixCmd.Flags().Lookup("exclude"))
	assert.NotNil(t, extensionFormat.Flags().Lookup("exclude"))
}

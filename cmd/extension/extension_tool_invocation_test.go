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
	selected, err := all.Only(all[0].Name())
	require.NoError(t, err)

	statuses := extensionToolInvocationStatuses(all, selected)
	assert.Len(t, statuses, len(all))
	assert.Equal(t, "invoked", toolStatusByName(t, statuses, all[0].Name()).Status)
	assert.Equal(t, "skipped", toolStatusByName(t, statuses, all[1].Name()).Status)
	assert.Equal(t, "not selected by --only", toolStatusByName(t, statuses, all[1].Name()).Reason)
}

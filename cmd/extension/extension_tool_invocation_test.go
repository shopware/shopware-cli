package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/verifier"
)

func TestExtensionToolInvocationStatuses(t *testing.T) {
	for _, all := range []verifier.ToolList{
		verifier.GetToolsOf[verifier.FixTool](),
		verifier.GetToolsOf[verifier.FormatTool](),
	} {
		selected, err := all.Only(all[0].Name())
		require.NoError(t, err)

		statuses := extensionToolInvocationStatuses(all, selected)
		assert.Len(t, statuses, len(all))
		assert.Equal(t, "invoked", toolStatusByName(t, statuses, all[0].Name()).Status)
		assert.Equal(t, "skipped", toolStatusByName(t, statuses, all[1].Name()).Status)
		assert.Equal(t, "not selected by --only", toolStatusByName(t, statuses, all[1].Name()).Reason)
	}
}

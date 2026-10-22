package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/verifier"
)

type invocationTestTool string

func (tool invocationTestTool) Name() string { return string(tool) }

func TestExtensionToolInvocationStatuses(t *testing.T) {
	assertExtensionToolInvocationStatuses(t, verifier.ToolList[verifier.Tool]{
		invocationTestTool("first"), invocationTestTool("second"), invocationTestTool("third"),
	})
}

func assertExtensionToolInvocationStatuses[T verifier.Tool](t *testing.T, all verifier.ToolList[T]) {
	t.Helper()
	requested, err := all.Only(all[0].Name() + "," + all[1].Name())
	require.NoError(t, err)
	selected, err := requested.Exclude(all[1].Name())
	require.NoError(t, err)

	_, statuses, selectionErr := verifier.SelectTools(all, all[0].Name()+","+all[1].Name(), all[1].Name(), "tools")
	require.NoError(t, selectionErr)
	assert.Len(t, selected, 1)
	assert.Len(t, statuses, len(all)-1)
	assert.Equal(t, "skipped", toolStatusByName(t, statuses, all[1].Name()).Status())
	assert.Equal(t, "excluded by --exclude", toolStatusByName(t, statuses, all[1].Name()).SkipReason)
	assert.Equal(t, "not selected by --only", toolStatusByName(t, statuses, all[2].Name()).SkipReason)

	_, err = requested.Exclude(all[2].Name())
	require.ErrorContains(t, err, "not found")
}

func TestExtensionFixAndFormatHaveExcludeFlag(t *testing.T) {
	assert.NotNil(t, extensionFixCmd.Flags().Lookup("exclude"))
	assert.NotNil(t, extensionFormat.Flags().Lookup("exclude"))
}

func toolStatusByName(
	t *testing.T,
	statuses []verifier.ToolResult,
	name string,
) verifier.ToolResult {
	t.Helper()

	for _, status := range statuses {
		if status.Name == name {
			return status
		}
	}

	t.Fatalf("missing tool status for %s", name)
	return verifier.ToolResult{}
}

package account

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/tui"
)

func TestAccountExtensionListFormatFlags(t *testing.T) {
	format := accountCompanyProducerExtensionListCmd.Flags().Lookup("format")
	require.NotNil(t, format)
	assert.False(t, format.Hidden)
	assert.Empty(t, format.Deprecated)
	assert.Equal(t, "table", format.DefValue)

	jsonAlias := accountCompanyProducerExtensionListCmd.Flags().Lookup("json")
	require.NotNil(t, jsonAlias)
	assert.True(t, jsonAlias.Hidden)
	assert.NotEmpty(t, jsonAlias.Deprecated)

	format.Changed = true
	jsonAlias.Changed = true
	t.Cleanup(func() {
		format.Changed = false
		jsonAlias.Changed = false
	})
	assert.Error(t, accountCompanyProducerExtensionListCmd.ValidateFlagGroups())
}

func TestAccountExtensionListDeprecatedJSONAlias(t *testing.T) {
	format, err := accountExtensionListFormat("table", true)
	require.NoError(t, err)
	assert.Equal(t, tui.TableFormatJSON, format)
}

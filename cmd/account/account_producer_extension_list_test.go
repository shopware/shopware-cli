package account

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountProducerExtensionListRejectsPluginAndApp(t *testing.T) {
	accountRootCmd.SetContext(t.Context())
	accountRootCmd.SetArgs([]string{"producer", "extension", "list", "--plugin", "--app"})
	// pflag keeps Changed=true across Execute calls, so without this reset
	// every later list execution in the test binary fails group validation.
	t.Cleanup(func() {
		accountRootCmd.SetArgs(nil)
		flags := accountCompanyProducerExtensionListCmd.Flags()
		for _, name := range []string{"plugin", "app"} {
			require.NoError(t, flags.Set(name, "false"))
			flags.Lookup(name).Changed = false
		}
	})

	err := accountRootCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "none of the others can be")
}

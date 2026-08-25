package account

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterWiresServiceContainer(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_CACHE_DIR", t.TempDir())
	t.Setenv("SHOPWARE_CLI_ACCOUNT_STAGING", "")

	var gotCommand string
	container := &ServiceContainer{}
	root := &cobra.Command{Use: "shopware-cli"}
	Register(root, func(commandName string) (*ServiceContainer, error) {
		gotCommand = commandName
		return container, nil
	})
	root.SetArgs([]string{"account", "logout"})
	// Cobra routes any later Execute on accountRootCmd through its root, so
	// the dummy parent must be detached again or it breaks the other tests.
	t.Cleanup(func() {
		root.SetArgs(nil)
		root.RemoveCommand(accountRootCmd)
		accountRootCmd.PersistentPreRunE = nil
		services = nil
	})

	require.NoError(t, root.Execute())
	assert.Equal(t, "logout", gotCommand)
	assert.Same(t, container, services)
}

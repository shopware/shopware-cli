package hub

import (
	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
)

var hubRootCmd = &cobra.Command{
	Use:   "hub",
	Short: "Shopware Hub commands",
}

// ServiceContainer holds the dependencies for hub commands.
type ServiceContainer struct {
	AccountClient *account_api.Client
}

var services *ServiceContainer

// Register adds the hub command tree to rootCmd. The onInit callback is
// called before each hub sub-command runs and must return an initialised
// ServiceContainer.
func Register(rootCmd *cobra.Command, onInit func() (*ServiceContainer, error)) {
	hubRootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		ser, err := onInit()
		services = ser
		return err
	}
	rootCmd.AddCommand(hubRootCmd)
}

package account

import (
	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
)

var accountRootCmd = &cobra.Command{
	Use:   "account",
	Short: "Authenticate with Shopware Account and upload extensions",
}

type ServiceContainer struct {
	AccountClient *account_api.Client
}

var services *ServiceContainer

func Register(rootCmd *cobra.Command, onInit func(commandName string) (*ServiceContainer, error)) {
	accountRootCmd.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		ser, err := onInit(cmd.Name())
		services = ser
		return err
	}
	rootCmd.AddCommand(accountRootCmd)
}

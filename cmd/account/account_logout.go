package account

import (
	"fmt"

	"github.com/spf13/cobra"

	accountApi "github.com/shopware/shopware-cli/account-api"
	"github.com/shopware/shopware-cli/logging"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Logout from Shopware Account",
	Long:  ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		err := accountApi.InvalidateTokenCache()
		if err != nil {
			return fmt.Errorf("cannot invalidate token cache: %w", err)
		}

		_ = services.Conf.SetAccountCompanyId(0)
		_ = services.Conf.SetAccountEmail("")
		_ = services.Conf.SetAccountPassword("")

		if err := services.Conf.Save(); err != nil {
			return fmt.Errorf("cannot write config: %w", err)
		}

		logging.FromContext(cmd.Context()).Infof("You have been logged out")

		return nil
	},
}

func init() {
	accountRootCmd.AddCommand(logoutCmd)
}

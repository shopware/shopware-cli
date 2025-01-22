package account

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	accountApi "github.com/shopware/shopware-cli/account-api"
	"github.com/shopware/shopware-cli/logging"
)

var accountCompanyUseCmd = &cobra.Command{
	Use:   "use [companyId]",
	Short: "Use another company for your Account",
	Args:  cobra.MinimumNArgs(1),
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		companyID, err := strconv.Atoi(args[0])
		if err != nil {
			return err
		}

		for _, membership := range services.AccountClient.GetMemberships() {
			if membership.Company.Id == companyID {
				if err := services.Conf.SetAccountCompanyId(companyID); err != nil {
					return err
				}

				if err := services.Conf.Save(); err != nil {
					return err
				}

				err = accountApi.InvalidateTokenCache()
				if err != nil {
					return fmt.Errorf("cannot invalidate token cache: %w", err)
				}

				logging.FromContext(cmd.Context()).Infof("Successfully changed your company to %s (%s)", membership.Company.Name, membership.Company.CustomerNumber)
				return nil
			}
		}

		return fmt.Errorf("company with ID \"%d\" not found", companyID)
	},
}

func init() {
	accountCompanyRootCmd.AddCommand(accountCompanyUseCmd)
}

package account

import (
	"fmt"

	"github.com/spf13/cobra"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Login into your Shopware Account",
	Long:  "",
	RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := accountApi.NewApi(cmd.Context(), nil)
		if err != nil {
			return err
		}

		fmt.Println("Client: ", client)
		return nil
	},
}

func init() {
	accountRootCmd.AddCommand(loginCmd)
}

// func changeAPIMembership(ctx context.Context, client *accountApi.Client, companyID int) error {
// 	if companyID == 0 || client.GetActiveCompanyID() == companyID {
// 		logging.FromContext(ctx).Debugf("Client is on correct membership skip")
// 		return nil
// 	}

// 	for _, membership := range client.GetMemberships() {
// 		if membership.Company.Id == companyID {
// 			logging.FromContext(ctx).Debugf("Changing member ship from %s (%d) to %s (%d)", client.ActiveMembership.Company.Name, client.ActiveMembership.Company.Id, membership.Company.Name, membership.Company.Id)
// 			return client.ChangeActiveMembership(ctx, membership)
// 		}
// 	}

// 	return fmt.Errorf("could not find configured company with id %d", companyID)
// }

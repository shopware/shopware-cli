package account

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/logging"
)

var accountProducerInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "List information about your producer account",
	Long:  ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get producer endpoint: %w", err)
		}

		profile, err := p.Profile(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get producer profile: %w", err)
		}

		logging.FromContext(cmd.Context()).Infof("Name: %s", profile.Name)
		logging.FromContext(cmd.Context()).Infof("Prefix: %s", profile.Prefix)
		logging.FromContext(cmd.Context()).Infof("Website: %s", profile.Website)

		return nil
	},
}

func init() {
	accountCompanyProducerCmd.AddCommand(accountProducerInfoCmd)
}

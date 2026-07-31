package account

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/account/producer"
)

var accountCompanyProducerExtensionInfoPushCmd = &cobra.Command{
	Use:   "push [zip or path]",
	Short: "Update store information of extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		zipExt, err := producer.ResolveExtension(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("cannot open extension: %w", err)
		}

		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get producer endpoint: %w", err)
		}

		return producer.PushStoreInfo(cmd.Context(), p, zipExt)
	},
}

func init() {
	accountCompanyProducerExtensionInfoCmd.AddCommand(accountCompanyProducerExtensionInfoPushCmd)
}

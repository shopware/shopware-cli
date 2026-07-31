package account

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/account/producer"
	"github.com/shopware/shopware-cli/internal/extension"
)

var accountCompanyProducerExtensionInfoPullCmd = &cobra.Command{
	Use:   "pull [path]",
	Short: "Generates local store configuration from account data",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absolutePath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot open file: %w", err)
		}

		zipExt, err := extension.GetExtensionByFolder(cmd.Context(), absolutePath)
		if err != nil {
			return fmt.Errorf("cannot open extension: %w", err)
		}

		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get producer endpoint: %w", err)
		}

		return producer.PullStoreInfo(cmd.Context(), p, zipExt, producer.PullOptions{})
	},
}

func init() {
	accountCompanyProducerExtensionInfoCmd.AddCommand(accountCompanyProducerExtensionInfoPullCmd)
}

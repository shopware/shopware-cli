package account

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
)

var accountCompanyProducerExtensionInfoPushCmd = &cobra.Command{
	Use:   "push [zip or path]",
	Short: "Update store information of extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absolutePath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("cannot open file: %w", err)
		}

		stat, err := os.Stat(absolutePath)
		if err != nil {
			return fmt.Errorf("cannot open file: %w", err)
		}

		var zipExt extension.Extension

		if stat.IsDir() {
			zipExt, err = extension.GetExtensionByFolder(cmd.Context(), absolutePath)
		} else {
			zipExt, err = extension.GetExtensionByZip(cmd.Context(), absolutePath)
		}

		if err != nil {
			return fmt.Errorf("cannot open extension: %w", err)
		}

		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get producer endpoint: %w", err)
		}

		return account_api.PushExtensionStoreInfo(cmd.Context(), p, zipExt)
	},
}

func init() {
	accountCompanyProducerExtensionInfoCmd.AddCommand(accountCompanyProducerExtensionInfoPushCmd)
}

package account

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/extension"
)

var accountCompanyProducerExtensionUploadCmd = &cobra.Command{
	Use:   "upload [zip]",
	Short: "Uploads a new extension version",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("validate: %w", err)
		}

		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return err
		}

		zipExt, err := extension.GetExtensionByZip(cmd.Context(), path)
		if err != nil {
			return err
		}

		return account_api.UploadExtension(cmd.Context(), p, zipExt, path, account_api.UploadOptions{
			SkipReviewWait: skipWaitingForCodereviewResult,
		})
	},
}

var skipWaitingForCodereviewResult bool

func init() {
	accountCompanyProducerExtensionCmd.AddCommand(accountCompanyProducerExtensionUploadCmd)
	accountCompanyProducerExtensionUploadCmd.Flags().BoolVar(&skipWaitingForCodereviewResult, "skip-for-review-result", false, "Skips waiting for Code review result")
}

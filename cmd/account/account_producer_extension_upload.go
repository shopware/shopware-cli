package account

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/account/producer"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/logging"
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

		logging.FromContext(cmd.Context()).Debugf("Starting upload process for file: %s", path)

		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to get producer client: %v", err)
			return err
		}

		zipExt, err := extension.GetExtensionByZip(cmd.Context(), path)
		if err != nil {
			logging.FromContext(cmd.Context()).Debugf("Failed to read extension from zip: %v", err)
			return err
		}

		opts := producer.NewUploadOptions()
		opts.SkipWaitingForReviewResult = skipWaitingForCodereviewResult

		return producer.Upload(cmd.Context(), p, zipExt, path, opts)
	},
}

var skipWaitingForCodereviewResult bool

func init() {
	accountCompanyProducerExtensionCmd.AddCommand(accountCompanyProducerExtensionUploadCmd)
	accountCompanyProducerExtensionUploadCmd.Flags().BoolVar(&skipWaitingForCodereviewResult, "skip-for-review-result", false, "Skips waiting for Code review result")
}

package account

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/account/producer"
)

var accountCompanyProducerExtensionListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all your extensions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get producer endpoint: %w", err)
		}

		return producer.ListExtensions(cmd.Context(), p, os.Stdout, producer.ListOptions{
			Search:     listExtensionSearch,
			PluginOnly: listExtensionPlugin,
			AppOnly:    listExtensionApp,
			JSON:       listExtensionJSON,
		})
	},
}

var (
	listExtensionSearch string
	listExtensionPlugin bool
	listExtensionApp    bool
	listExtensionJSON   bool
)

func init() {
	accountCompanyProducerExtensionCmd.AddCommand(accountCompanyProducerExtensionListCmd)
	accountCompanyProducerExtensionListCmd.Flags().StringVar(&listExtensionSearch, "search", "", "Filter for name")
	accountCompanyProducerExtensionListCmd.Flags().BoolVar(&listExtensionPlugin, "plugin", false, "Show only plugins")
	accountCompanyProducerExtensionListCmd.Flags().BoolVar(&listExtensionApp, "app", false, "Show only apps")
	accountCompanyProducerExtensionListCmd.Flags().BoolVar(&listExtensionJSON, "json", false, "Output as json")
	accountCompanyProducerExtensionListCmd.MarkFlagsMutuallyExclusive("plugin", "app")
}

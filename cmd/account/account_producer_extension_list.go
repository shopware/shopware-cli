package account

import (
	"os"

	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
)

var accountCompanyProducerExtensionListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all your extensions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return err
		}

		extensions, err := account_api.ListProducerExtensions(cmd.Context(), p, account_api.ListExtensionOptions{
			Search:     listExtensionSearch,
			PluginOnly: listExtensionPlugin,
			AppOnly:    listExtensionApp,
		})
		if err != nil {
			return err
		}

		if listExtensionJSON {
			return account_api.WriteExtensionsJSON(os.Stdout, extensions)
		}

		return account_api.WriteExtensionsTable(os.Stdout, extensions)
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

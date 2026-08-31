package account

import (
	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/tui"
)

var accountCompanyProducerExtensionListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all your extensions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		format, err := tui.ParseTableFormat(listExtensionFormat)
		if err != nil {
			return err
		}
		if listExtensionJSON {
			format = tui.TableFormatJSON
		}

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

		out := cmd.OutOrStdout()
		return account_api.ExtensionsTable(extensions).Write(out, format)
	},
}

var (
	listExtensionSearch string
	listExtensionPlugin bool
	listExtensionApp    bool
	listExtensionFormat string
	listExtensionJSON   bool
)

func init() {
	accountCompanyProducerExtensionCmd.AddCommand(accountCompanyProducerExtensionListCmd)
	accountCompanyProducerExtensionListCmd.Flags().StringVar(&listExtensionSearch, "search", "", "Filter for name")
	accountCompanyProducerExtensionListCmd.Flags().BoolVar(&listExtensionPlugin, "plugin", false, "Show only plugins")
	accountCompanyProducerExtensionListCmd.Flags().BoolVar(&listExtensionApp, "app", false, "Show only apps")
	accountCompanyProducerExtensionListCmd.Flags().StringVar(&listExtensionFormat, "format", string(tui.TableFormatTable), "Output format (table or json)")
	accountCompanyProducerExtensionListCmd.Flags().BoolVar(&listExtensionJSON, "json", false, "Output as json")
	accountCompanyProducerExtensionListCmd.MarkFlagsMutuallyExclusive("plugin", "app")
	accountCompanyProducerExtensionListCmd.MarkFlagsMutuallyExclusive("format", "json")
}

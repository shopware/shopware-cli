package account

import (
	"fmt"
	"os"
	"strconv"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/account-api"
)

var accountCompanyProducerExtensionListCmd = &cobra.Command{
	Use:   "list",
	Short: "Lists all your extensions",
	RunE: func(cmd *cobra.Command, _ []string) error {
		p, err := services.AccountClient.Producer(cmd.Context())
		if err != nil {
			return fmt.Errorf("cannot get producer endpoint: %w", err)
		}

		criteria := account_api.ListExtensionCriteria{
			Limit: 100,
		}

		if len(listExtensionSearch) > 0 {
			criteria.Search = listExtensionSearch
			criteria.OrderBy = "name"
			criteria.OrderSequence = "asc"
		}

		extensions, err := p.Extensions(cmd.Context(), &criteria)
		if err != nil {
			return err
		}

		table := tablewriter.NewWriter(os.Stdout)
		table.SetHeader([]string{"ID", "Name", "Type", "Compatible with latest version", "Status"})

		for _, extension := range extensions {
			if extension.Status.Name == "deleted" {
				continue
			}

			compatible := "No"

			if extension.IsCompatibleWithLatestShopwareVersion {
				compatible = "Yes"
			}

			table.Append([]string{
				strconv.FormatInt(int64(extension.Id), 10),
				extension.Name,
				extension.Generation.Description,
				compatible,
				extension.Status.Name,
			})
		}

		table.Render()

		return nil
	},
}

var listExtensionSearch string

func init() {
	accountCompanyProducerExtensionCmd.AddCommand(accountCompanyProducerExtensionListCmd)
	accountCompanyProducerExtensionListCmd.Flags().StringVar(&listExtensionSearch, "search", "", "Filter for name")
}

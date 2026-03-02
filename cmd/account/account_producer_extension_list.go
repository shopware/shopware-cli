package account

import (
	"fmt"
	"strconv"

	"charm.land/lipgloss/v2"
	liplogtable "charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
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

		t := liplogtable.New().
			Border(lipgloss.NormalBorder()).
			Headers("ID", "Name", "Type", "Compatible with latest version", "Status")

		for _, extension := range extensions {
			if extension.Status.Name == "deleted" {
				continue
			}

			compatible := "No"
			if extension.IsCompatibleWithLatestShopwareVersion {
				compatible = "Yes"
			}

			t.Row(
				strconv.FormatInt(int64(extension.Id), 10),
				extension.Name,
				extension.Generation.Description,
				compatible,
				extension.Status.Name,
			)
		}

		fmt.Println(t.Render())

		return nil
	},
}

var listExtensionSearch string

func init() {
	accountCompanyProducerExtensionCmd.AddCommand(accountCompanyProducerExtensionListCmd)
	accountCompanyProducerExtensionListCmd.Flags().StringVar(&listExtensionSearch, "search", "", "Filter for name")
}

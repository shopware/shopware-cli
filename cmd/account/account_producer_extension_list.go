package account

import (
	"cmp"
	"fmt"
	"slices"

	"charm.land/lipgloss/v2"
	liplogtable "charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/color"
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

		slices.SortFunc(extensions, func(a, b account_api.Extension) int {
			if c := cmp.Compare(a.Producer.Name, b.Producer.Name); c != 0 {
				return c
			}
			return cmp.Compare(a.Name, b.Name)
		})

		cellStyle := lipgloss.NewStyle().Padding(0, 1)

		t := liplogtable.New().
			Border(lipgloss.NormalBorder()).
			StyleFunc(func(row, col int) lipgloss.Style {
				return cellStyle
			}).
			Headers("Name", "Type", "Compatible with latest version", "Status")

		lastProducerId := 0
		for _, extension := range extensions {
			if extension.Status.Name == "deleted" || extension.Name == "" || extension.Generation.Name == "classic" {
				continue
			}

			if extension.Producer.Id != lastProducerId {
				lastProducerId = extension.Producer.Id
				t.Row(color.BoldText.Render(extension.Producer.Name), "", "", "")
			}

			compatible := color.RedText.Render("No")
			if extension.IsCompatibleWithLatestShopwareVersion {
				compatible = color.GreenText.Render("Yes")
			}

			var status string
			switch extension.Status.Name {
			case "instore", "approved":
				status = color.GreenText.Render(extension.Status.Name)
			case "incomplete", "waitingforapproval":
				status = color.YellowText.Render(extension.Status.Name)
			default:
				status = color.DimText.Render(extension.Status.Name)
			}

			t.Row(
				"  "+extension.Name,
				color.DimText.Render(extension.Generation.Description),
				compatible,
				status,
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

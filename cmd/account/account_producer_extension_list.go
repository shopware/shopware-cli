package account

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"

	"charm.land/lipgloss/v2"
	liplogtable "charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/tui"
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

		if listExtensionSearch != "" {
			criteria.Search = listExtensionSearch
			criteria.OrderBy = "name"
			criteria.OrderSequence = "asc"
		}

		extensions, err := p.Extensions(cmd.Context(), &criteria)
		if err != nil {
			return err
		}

		extensions = filterProducerExtensions(extensions, listExtensionPlugin, listExtensionApp)

		slices.SortFunc(extensions, func(a, b account_api.Extension) int {
			if c := cmp.Compare(a.Producer.Name, b.Producer.Name); c != 0 {
				return c
			}
			return cmp.Compare(a.Name, b.Name)
		})

		if listExtensionJSON {
			return printProducerExtensionsJSON(extensions)
		}

		return printProducerExtensionsTable(extensions)
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

// filterProducerExtensions removes deleted/classic/empty entries and optionally
// keeps only plugins or apps.
func filterProducerExtensions(extensions []account_api.Extension, pluginOnly, appOnly bool) []account_api.Extension {
	filtered := make([]account_api.Extension, 0, len(extensions))
	for _, extension := range extensions {
		if !includeProducerExtension(extension, pluginOnly, appOnly) {
			continue
		}
		filtered = append(filtered, extension)
	}
	return filtered
}

func includeProducerExtension(extension account_api.Extension, pluginOnly, appOnly bool) bool {
	if extension.Status.Name == "deleted" || extension.Name == "" || extension.Generation.Name == account_api.ExtensionGenerationClassic {
		return false
	}
	if pluginOnly && extension.Generation.Name != account_api.ExtensionGenerationPlatform {
		return false
	}
	if appOnly && extension.Generation.Name != account_api.ExtensionGenerationApps {
		return false
	}
	return true
}

type producerExtensionListItem struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Compatible bool   `json:"compatibleWithLatestVersion"`
	Status     string `json:"status"`
	Producer   string `json:"producer"`
}

func printProducerExtensionsJSON(extensions []account_api.Extension) error {
	items := make([]producerExtensionListItem, 0, len(extensions))
	for _, extension := range extensions {
		items = append(items, producerExtensionListItem{
			Name:       extension.Name,
			Type:       extension.Generation.Name,
			Compatible: extension.IsCompatibleWithLatestShopwareVersion,
			Status:     extension.Status.Name,
			Producer:   extension.Producer.Name,
		})
	}

	content, err := json.Marshal(items)
	if err != nil {
		return err
	}

	fmt.Println(string(content))
	return nil
}

func printProducerExtensionsTable(extensions []account_api.Extension) error {
	cellStyle := lipgloss.NewStyle().Padding(0, 1)

	t := liplogtable.New().
		Border(lipgloss.NormalBorder()).
		StyleFunc(func(row, col int) lipgloss.Style {
			return cellStyle
		}).
		Headers("Name", "Type", "Compatible with latest version", "Status")

	lastProducerId := 0
	for _, extension := range extensions {
		if extension.Producer.Id != lastProducerId {
			lastProducerId = extension.Producer.Id
			t.Row(tui.BoldText.Render(extension.Producer.Name), "", "", "")
		}

		compatible := tui.RedText.Render("No")
		if extension.IsCompatibleWithLatestShopwareVersion {
			compatible = tui.GreenText.Render("Yes")
		}

		var status string
		switch extension.Status.Name {
		case "instore", "approved":
			status = tui.GreenText.Render(extension.Status.Name)
		case "incomplete", "waitingforapproval":
			status = tui.YellowText.Render(extension.Status.Name)
		default:
			status = tui.DimText.Render(extension.Status.Name)
		}

		t.Row(
			"  "+extension.Name,
			tui.DimText.Render(extension.Generation.Description),
			compatible,
			status,
		)
	}

	fmt.Println(t.Render())
	return nil
}

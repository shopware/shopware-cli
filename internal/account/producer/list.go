package producer

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"charm.land/lipgloss/v2"
	liplogtable "charm.land/lipgloss/v2/table"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/tui"
)

// ListOptions controls filtering and rendering of ListExtensions.
type ListOptions struct {
	Search     string
	PluginOnly bool
	AppOnly    bool
	JSON       bool
}

// ListExtensions fetches all producer extensions, filters and sorts them and
// renders the result as table or JSON to the given writer.
func ListExtensions(ctx context.Context, api ListAPI, w io.Writer, opts ListOptions) error {
	criteria := accountApi.ListExtensionCriteria{
		Limit: 100,
	}

	if opts.Search != "" {
		criteria.Search = opts.Search
		criteria.OrderBy = "name"
		criteria.OrderSequence = "asc"
	}

	extensions, err := api.Extensions(ctx, &criteria)
	if err != nil {
		return err
	}

	extensions = filterExtensions(extensions, opts.PluginOnly, opts.AppOnly)

	slices.SortFunc(extensions, func(a, b accountApi.Extension) int {
		if c := cmp.Compare(a.Producer.Name, b.Producer.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	if opts.JSON {
		return printExtensionsJSON(w, extensions)
	}

	return printExtensionsTable(w, extensions)
}

// filterExtensions removes deleted/classic/empty entries and optionally
// keeps only plugins or apps.
func filterExtensions(extensions []accountApi.Extension, pluginOnly, appOnly bool) []accountApi.Extension {
	filtered := make([]accountApi.Extension, 0, len(extensions))
	for _, extension := range extensions {
		if !includeExtension(extension, pluginOnly, appOnly) {
			continue
		}
		filtered = append(filtered, extension)
	}
	return filtered
}

func includeExtension(extension accountApi.Extension, pluginOnly, appOnly bool) bool {
	if extension.Status.Name == "deleted" || extension.Name == "" || extension.Generation.Name == accountApi.ExtensionGenerationClassic {
		return false
	}
	if pluginOnly && extension.Generation.Name != accountApi.ExtensionGenerationPlatform {
		return false
	}
	if appOnly && extension.Generation.Name != accountApi.ExtensionGenerationApps {
		return false
	}
	return true
}

type extensionListItem struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Compatible bool   `json:"compatibleWithLatestVersion"`
	Status     string `json:"status"`
	Producer   string `json:"producer"`
}

func printExtensionsJSON(w io.Writer, extensions []accountApi.Extension) error {
	items := make([]extensionListItem, 0, len(extensions))
	for _, extension := range extensions {
		items = append(items, extensionListItem{
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

	fmt.Fprintln(w, string(content))
	return nil
}

func printExtensionsTable(w io.Writer, extensions []accountApi.Extension) error {
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

	fmt.Fprintln(w, t.Render())
	return nil
}

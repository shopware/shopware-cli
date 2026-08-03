package account_api

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"

	"charm.land/lipgloss/v2"
	liplogtable "charm.land/lipgloss/v2/table"

	"github.com/shopware/shopware-cli/internal/tui"
)

type ListExtensionOptions struct {
	Search     string
	PluginOnly bool
	AppOnly    bool
}

// ListProducerExtensions fetches the extensions of the current producer, filters out
// deleted/classic/empty entries and sorts them by producer and name.
func ListProducerExtensions(ctx context.Context, producer ProducerAPI, opts ListExtensionOptions) ([]Extension, error) {
	criteria := ListExtensionCriteria{
		Limit: 100,
	}

	if opts.Search != "" {
		criteria.Search = opts.Search
		criteria.OrderBy = "name"
		criteria.OrderSequence = "asc"
	}

	extensions, err := producer.Extensions(ctx, &criteria)
	if err != nil {
		return nil, err
	}

	extensions = FilterExtensions(extensions, opts.PluginOnly, opts.AppOnly)

	slices.SortFunc(extensions, func(a, b Extension) int {
		if c := cmp.Compare(a.Producer.Name, b.Producer.Name); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	return extensions, nil
}

// FilterExtensions removes deleted/classic/empty entries and optionally
// keeps only plugins or apps.
func FilterExtensions(extensions []Extension, pluginOnly, appOnly bool) []Extension {
	filtered := make([]Extension, 0, len(extensions))
	for _, extension := range extensions {
		if !IncludeExtension(extension, pluginOnly, appOnly) {
			continue
		}
		filtered = append(filtered, extension)
	}
	return filtered
}

func IncludeExtension(extension Extension, pluginOnly, appOnly bool) bool {
	if extension.Status.Name == "deleted" || extension.Name == "" || extension.Generation.Name == ExtensionGenerationClassic {
		return false
	}
	if pluginOnly && extension.Generation.Name != ExtensionGenerationPlatform {
		return false
	}
	if appOnly && extension.Generation.Name != ExtensionGenerationApps {
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

func WriteExtensionsJSON(w io.Writer, extensions []Extension) error {
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

	_, err = fmt.Fprintln(w, string(content))
	return err
}

func WriteExtensionsTable(w io.Writer, extensions []Extension) error {
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

	_, err := fmt.Fprintln(w, t.Render())
	return err
}

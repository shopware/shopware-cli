package account_api

import (
	"cmp"
	"context"
	"io"
	"slices"

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

	// Fetch all pages; a producer with more than one page of extensions would otherwise be
	// listed incompletely.
	extensions := make([]Extension, 0)
	for criteria.Offset = 0; ; criteria.Offset += criteria.Limit {
		page, err := producer.Extensions(ctx, &criteria)
		if err != nil {
			return nil, err
		}

		extensions = append(extensions, page...)

		if len(page) < criteria.Limit {
			break
		}
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

func ExtensionsTable(extensions []Extension) *tui.Table {
	result := tui.NewTable(
		tui.TableColumn{Title: "Name", JSONKey: "name"},
		tui.TableColumn{Title: "Type", JSONKey: "type"},
		tui.TableColumn{Title: "Compatible with latest version", JSONKey: "compatibleWithLatestVersion"},
		tui.TableColumn{Title: "Status", JSONKey: "status"},
		tui.TableColumn{Title: "Producer", JSONKey: "producer"},
	)

	for _, extension := range extensions {
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

		result.AddRow(
			extension.Name,
			tui.TableCell{
				Value:        extension.Generation.Name,
				TerminalText: tui.DimText.Render(extension.Generation.Description),
			},
			tui.TableCell{
				Value:        extension.IsCompatibleWithLatestShopwareVersion,
				TerminalText: compatible,
			},
			tui.TableCell{
				Value:        extension.Status.Name,
				TerminalText: status,
			},
			extension.Producer.Name,
		)
	}

	return result
}

func WriteExtensionsJSON(w io.Writer, extensions []Extension) error {
	return ExtensionsTable(extensions).Write(w, tui.TableFormatJSON)
}

func WriteExtensionsTable(w io.Writer, extensions []Extension) error {
	return ExtensionsTable(extensions).Write(w, tui.TableFormatTable)
}

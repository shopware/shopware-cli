package ai

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/tui"
)

// listItem is the per-entry JSON shape for `ai list` (a subset of the info
// shape). Field names are the public contract; see the directory CONTRACT.md.
type listItem struct {
	Name        string           `json:"name"`
	DisplayName string           `json:"displayName"`
	Type        directory.Type   `json:"type"`
	Provider    string           `json:"provider"`
	Description string           `json:"description"`
	Status      directory.Status `json:"status"`
}

var aiListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List known Shopware AI integrations",
	Long: `List the known Shopware AI integrations.

With --installed, list the integrations this CLI has installed instead — one row
per agent and scope, with the requested tag and resolved revision.`,
	Example: `  shopware-cli ai list
  shopware-cli ai list --installed
  shopware-cli ai list --format json`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		format, err := resolveFormat(cmd)
		if err != nil {
			return err
		}

		typeFilter, _ := cmd.Flags().GetString("type")
		installedOnly, _ := cmd.Flags().GetBool("installed")

		// List validates the type filter (for both branches) and yields the
		// type-filtered catalog.
		entries, err := directory.Load().List(directory.ListOptions{Type: typeFilter})
		if err != nil {
			return err
		}

		// --installed reports the recorded installs (one row per agent/scope),
		// not the catalog, so it can show the agent, scope and revision.
		if installedOnly {
			records, err := installedRecords()
			if err != nil {
				return err
			}
			if typeFilter != "" {
				records = recordsInCatalog(records, entries)
			}

			return writeInstalled(cmd.OutOrStdout(), format, records)
		}

		return writeList(cmd.OutOrStdout(), format, entries)
	},
}

func writeList(w io.Writer, format string, entries []directory.Integration) error {
	t := tui.NewTable(
		tui.TableColumn{Title: "Name", JSONKey: "name"},
		tui.TableColumn{Title: "Type", JSONKey: "type"},
		tui.TableColumn{Title: "Provider", JSONKey: "provider"},
		tui.TableColumn{Title: "Status", JSONKey: "status"},
		tui.TableColumn{Title: "Description", JSONKey: "description"},
	)
	for _, e := range entries {
		t.AddRowWithJSON(listItem{
			Name:        e.Name,
			DisplayName: e.DisplayName,
			Type:        e.Type,
			Provider:    e.Provider,
			Description: e.Description,
			Status:      e.Status,
		}, e.Name, string(e.Type), e.Provider, string(e.Status), e.Description)
	}

	return t.Write(w, tui.TableFormat(format))
}

func writeInstalled(w io.Writer, format string, records []state.InstalledEntry) error {
	t := tui.NewTable(
		tui.TableColumn{Title: "Name", JSONKey: "name"},
		tui.TableColumn{Title: "Agent", JSONKey: "agent"},
		tui.TableColumn{Title: "Scope", JSONKey: "scope"},
		tui.TableColumn{Title: "Requested", JSONKey: "requestedTag"},
		tui.TableColumn{Title: "Revision", JSONKey: "resolvedRevision"},
	)
	for _, e := range records {
		t.AddRowWithJSON(e, e.Name, e.Agent, string(e.Scope), e.RequestedTag, e.ResolvedRevision)
	}

	return t.Write(w, tui.TableFormat(format))
}

// recordsInCatalog keeps only the install records whose integration is in the
// (type-filtered) catalog, so `ai list --installed --type X` shows only type X.
func recordsInCatalog(records []state.InstalledEntry, catalog []directory.Integration) []state.InstalledEntry {
	names := make(map[string]bool, len(catalog))
	for _, e := range catalog {
		names[e.Name] = true
	}

	out := make([]state.InstalledEntry, 0, len(records))
	for _, r := range records {
		if names[r.Name] {
			out = append(out, r)
		}
	}

	return out
}

// installedRecords returns every recorded install, merging global and project
// state. Each record carries its own agent and scope.
func installedRecords() ([]state.InstalledEntry, error) {
	global, err := state.Read()
	if err != nil {
		return nil, err
	}
	records := append([]state.InstalledEntry{}, global.Installed...)

	// Fall back to the current directory when not inside a project; global
	// installs still show.
	root, err := shop.FindClosestShopwareProject(true)
	if err != nil {
		return nil, fmt.Errorf("locate project install state: %w", err)
	}
	project, err := state.ReadProject(root)
	if err != nil {
		return nil, err
	}

	return append(records, project.Installed...), nil
}

// installedRecordsFor returns the recorded installs for a single integration.
func installedRecordsFor(name string) ([]state.InstalledEntry, error) {
	all, err := installedRecords()
	if err != nil {
		return nil, err
	}

	out := make([]state.InstalledEntry, 0, len(all))
	for _, e := range all {
		if e.Name == name {
			out = append(out, e)
		}
	}

	return out, nil
}

func init() {
	aiRootCmd.AddCommand(aiListCmd)
	aiListCmd.Flags().String("type", "", "Filter by integration type (skill, mcp)")
	aiListCmd.Flags().Bool("installed", false, "Show only integrations recorded as installed by the CLI")
	addFormatFlag(aiListCmd)
}

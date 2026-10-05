package ai

import (
	"encoding/json"
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

		// --installed reports the recorded installs (one row per agent/scope),
		// not the catalog, so it can show the agent, scope and revision.
		if installedOnly {
			records, err := installedRecords()
			if err != nil {
				return err
			}
			if format == formatJSON {
				return writeInstalledJSON(cmd.OutOrStdout(), records)
			}

			return writeInstalledTable(cmd.OutOrStdout(), records)
		}

		entries, err := directory.Load().List(nil, directory.ListOptions{Type: typeFilter})
		if err != nil {
			return err
		}

		if format == formatJSON {
			return writeListJSON(cmd.OutOrStdout(), entries)
		}

		return writeListTable(cmd.OutOrStdout(), entries)
	},
}

func writeListJSON(w io.Writer, entries []directory.Integration) error {
	items := make([]listItem, 0, len(entries))
	for _, e := range entries {
		items = append(items, listItem{
			Name:        e.Name,
			DisplayName: e.DisplayName,
			Type:        e.Type,
			Provider:    e.Provider,
			Description: e.Description,
			Status:      e.Status,
		})
	}

	out, err := json.Marshal(items)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, string(out))

	return err
}

func writeListTable(w io.Writer, entries []directory.Integration) error {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{e.Name, string(e.Type), e.Provider, string(e.Status), e.Description})
	}

	_, err := fmt.Fprintln(w, tui.RenderTable(
		[]string{"Name", "Type", "Provider", "Status", "Description"},
		rows,
	))

	return err
}

// installedRecords returns every install the CLI recorded, merging the global
// state and the project state at the Shopware project root (a missing file yields
// an empty state, not an error). Each record carries its own agent and scope, so
// the same integration installed for two agents or scopes yields two rows.
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

func writeInstalledJSON(w io.Writer, records []state.InstalledEntry) error {
	if records == nil {
		records = []state.InstalledEntry{}
	}

	out, err := json.Marshal(records)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, string(out))

	return err
}

func writeInstalledTable(w io.Writer, records []state.InstalledEntry) error {
	rows := make([][]string, 0, len(records))
	for _, e := range records {
		rows = append(rows, []string{e.Name, e.Agent, string(e.Scope), e.RequestedTag, e.ResolvedRevision})
	}

	_, err := fmt.Fprintln(w, tui.RenderTable(
		[]string{"Name", "Agent", "Scope", "Requested", "Revision"},
		rows,
	))

	return err
}

func init() {
	aiRootCmd.AddCommand(aiListCmd)
	aiListCmd.Flags().String("type", "", "Filter by integration type (skill, mcp)")
	aiListCmd.Flags().Bool("installed", false, "Show only integrations recorded as installed by the CLI")
	addFormatFlag(aiListCmd)
}

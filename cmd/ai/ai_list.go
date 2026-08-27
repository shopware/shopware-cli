package ai

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/tui"
)

// knownTypeFilters are the type identifiers accepted by --type. It includes
// "mcp", reserved for a future increment (#1336): it currently matches no
// entry, so `--type mcp` returns an empty list with exit code 0. Any other
// value is rejected as an error.
var knownTypeFilters = map[string]bool{
	string(directory.TypeSkill): true,
	"mcp":                       true,
}

// listItem is the per-entry JSON shape for `ai list` (a subset of the info
// shape). Field names are the public contract; see the directory CONTRACT.md.
type listItem struct {
	Name        string           `json:"name"`
	DisplayName string           `json:"displayName"`
	Type        directory.Type   `json:"type"`
	Provider    string           `json:"provider"`
	Description string           `json:"description"`
	Status      directory.Status `json:"status"`
	Available   bool             `json:"available"`
}

var aiListCmd = &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "List known Shopware AI integrations",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		typeFilter, err := cmd.Flags().GetString("type")
		if err != nil {
			return err
		}

		installedOnly, err := cmd.Flags().GetBool("installed")
		if err != nil {
			return err
		}

		asJSON, err := cmd.Flags().GetBool("json")
		if err != nil {
			return err
		}

		if typeFilter != "" && !knownTypeFilters[typeFilter] {
			return fmt.Errorf("unknown --type %q (allowed: skill)", typeFilter)
		}

		dir, err := directory.Load()
		if err != nil {
			return err
		}

		entries := make([]directory.Integration, 0, len(dir.Integrations))
		for _, e := range dir.Integrations {
			if typeFilter != "" && string(e.Type) != typeFilter {
				continue
			}
			// Nothing writes install state yet (#1337), so --installed is
			// legitimately empty for now.
			if installedOnly {
				continue
			}

			entries = append(entries, applyAvailability(e))
		}

		if asJSON {
			items := make([]listItem, 0, len(entries))
			for _, e := range entries {
				items = append(items, listItem{
					Name:        e.Name,
					DisplayName: e.DisplayName,
					Type:        e.Type,
					Provider:    e.Provider,
					Description: e.Description,
					Status:      e.Status,
					Available:   e.Available,
				})
			}

			out, err := json.Marshal(items)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))

			return err
		}

		rows := make([][]string, 0, len(entries))
		for _, e := range entries {
			rows = append(rows, []string{e.Name, string(e.Type), e.Provider, string(e.Status), availableLabel(e), e.Description})
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), tui.RenderTable(
			[]string{"Name", "Type", "Provider", "Status", "Available", "Description"},
			rows,
		))

		return err
	},
}

// availableLabel renders the availability of an entry for the human table.
func availableLabel(e directory.Integration) string {
	if e.Available {
		return "yes"
	}
	if e.AvailabilityReason != "" {
		return fmt.Sprintf("no (%s)", e.AvailabilityReason)
	}

	return "no"
}

func init() {
	aiRootCmd.AddCommand(aiListCmd)
	aiListCmd.Flags().String("type", "", "Filter by integration type (skill)")
	aiListCmd.Flags().Bool("installed", false, "Show only integrations recorded as installed by the CLI")
	aiListCmd.Flags().Bool("json", false, "Output as json")
}

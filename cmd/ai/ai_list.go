package ai

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
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

var aiListCmd = newAIListCmd()

func newAIListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "list",
		Aliases:      []string{"ls"},
		Short:        "List known Shopware AI integrations",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE:         runAIList,
	}

	cmd.Flags().String("type", "", "Filter by integration type (skill, mcp)")
	cmd.Flags().Bool("installed", false, "Show only integrations recorded as installed by the CLI")
	cmd.Flags().Bool("json", false, "Output as json")

	return cmd
}

func runAIList(cmd *cobra.Command, _ []string) error {
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
		return fmt.Errorf("unknown --type %q (allowed: skill, mcp)", typeFilter)
	}

	dir, err := directory.Load()
	if err != nil {
		return err
	}

	var installedNames map[string]bool
	if installedOnly {
		installedNames, err = readInstalledNames()
		if err != nil {
			return err
		}
	}

	entries := make([]directory.Integration, 0, len(dir.Integrations))
	for _, e := range dir.Integrations {
		if typeFilter != "" && string(e.Type) != typeFilter {
			continue
		}
		if installedOnly && !installedNames[e.Name] {
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
}

// readInstalledNames returns the set of integration names recorded as installed
// by the CLI. Nothing writes the state file until #1337, so today this is
// empty (a missing file yields an empty state, not an error).
func readInstalledNames() (map[string]bool, error) {
	path, err := state.Path()
	if err != nil {
		return nil, err
	}

	st, err := state.Read(path)
	if err != nil {
		return nil, err
	}

	names := make(map[string]bool, len(st.Installed))
	for _, e := range st.Installed {
		names[e.Name] = true
	}

	return names, nil
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
}

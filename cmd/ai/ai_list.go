package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
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
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "List known Shopware AI integrations",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		format, err := resolveFormat(cmd)
		if err != nil {
			return err
		}

		typeFilter, _ := cmd.Flags().GetString("type")
		installedOnly, _ := cmd.Flags().GetBool("installed")

		var installed map[string]bool
		if installedOnly {
			if installed, err = readInstalledNames(); err != nil {
				return err
			}
		}

		entries, err := directory.Load().List(installed, directory.ListOptions{
			Type:          typeFilter,
			InstalledOnly: installedOnly,
		})
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

// readInstalledNames returns the set of integration names recorded as installed
// by the CLI, merging the global state and the project state in the current
// directory (a missing file yields an empty state, not an error).
func readInstalledNames() (map[string]bool, error) {
	names := map[string]bool{}

	global, err := state.Read()
	if err != nil {
		return nil, err
	}
	for _, e := range global.Installed {
		names[e.Name] = true
	}

	// A project-scoped install is recorded in the current directory.
	if root, err := os.Getwd(); err == nil {
		project, err := state.ReadProject(root)
		if err != nil {
			return nil, err
		}
		for _, e := range project.Installed {
			names[e.Name] = true
		}
	}

	return names, nil
}

func init() {
	aiRootCmd.AddCommand(aiListCmd)
	aiListCmd.Flags().String("type", "", "Filter by integration type (skill, mcp)")
	aiListCmd.Flags().Bool("installed", false, "Show only integrations recorded as installed by the CLI")
	addFormatFlag(aiListCmd)
}

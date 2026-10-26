package ai

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/mcp"
	"github.com/shopware/shopware-cli/internal/ai/skills"
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

// installedRow is one line of `ai list --installed`: a skill (one row per scope)
// or an MCP server (always global). The JSON field names are the public contract.
type installedRow struct {
	Name   string         `json:"name"`
	Type   directory.Type `json:"type"`
	Agents []string       `json:"agents"`
	Scope  string         `json:"scope"`
	Source string         `json:"source"`
}

// Seams over the install state sources so tests can stub them.
var (
	listSkills = skills.List
	listMCP    = mcp.List
)

var aiListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List known Shopware AI integrations",
	Long: `List the known Shopware AI integrations.

With --installed, list what is installed instead: skills (as skills.sh reports
them, one row per skill and scope) and MCP servers (from the agent config files,
always global), each with its type, agents and source. Listing skills requires
Node.js/npx on PATH.`,
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

		if installedOnly {
			records, err := installedRecords(cmd.Context(), entries)
			if err != nil {
				return err
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
		}, e.Name, e.Type.String(), e.Provider, string(e.Status), e.Description)
	}

	return t.Write(w, tui.TableFormat(format))
}

func writeInstalled(w io.Writer, format string, records []installedRow) error {
	t := tui.NewTable(
		tui.TableColumn{Title: "Name", JSONKey: "name"},
		tui.TableColumn{Title: "Type", JSONKey: "type"},
		tui.TableColumn{Title: "Agents", JSONKey: "agents"},
		tui.TableColumn{Title: "Scope", JSONKey: "scope"},
		tui.TableColumn{Title: "Source", JSONKey: "source"},
	)
	for _, e := range records {
		t.AddRowWithJSON(e, e.Name, e.Type.String(), strings.Join(e.Agents, ", "), e.Scope, e.Source)
	}

	return t.Write(w, tui.TableFormat(format))
}

// installedRecords reports what is installed and keeps only the entries in the
// given catalog (already type-filtered). Skills come from skills.sh (global and
// the current project); MCP servers come from the agent config files and are
// always global.
func installedRecords(ctx context.Context, catalog []directory.Integration) ([]installedRow, error) {
	known := make(map[string]bool, len(catalog))
	for _, e := range catalog {
		known[e.Name] = true
	}

	global, err := listSkills(ctx, "", true)
	if err != nil {
		return nil, err
	}

	root, err := shop.FindClosestShopwareProject(true)
	if err != nil {
		return nil, fmt.Errorf("locate project install state: %w", err)
	}
	project, err := listSkills(ctx, root, false)
	if err != nil {
		return nil, err
	}

	out := make([]installedRow, 0, len(global)+len(project))
	for _, e := range append(global, project...) {
		if known[e.Name] {
			out = append(out, installedRow{
				Name:   e.Name,
				Type:   directory.TypeSkill,
				Agents: e.Agents,
				Scope:  e.Scope,
				Source: e.Source,
			})
		}
	}

	servers, err := listMCP(root)
	if err != nil {
		return nil, err
	}
	for _, s := range servers {
		if known[s.Name] {
			out = append(out, installedRow{
				Name:   s.Name,
				Type:   directory.TypeMCP,
				Scope:  "project",
				Source: s.Source,
			})
		}
	}

	return out, nil
}

func init() {
	aiRootCmd.AddCommand(aiListCmd)
	aiListCmd.Flags().String("type", "", "Filter by integration type (skill, mcp)")
	_ = aiListCmd.RegisterFlagCompletionFunc("type", cobra.FixedCompletions([]string{directory.TypeSkill.String(), directory.TypeMCP.String()}, cobra.ShellCompDirectiveNoFileComp))
	aiListCmd.Flags().Bool("installed", false, "Show installed skills and MCP servers")
	addFormatFlag(aiListCmd)
}

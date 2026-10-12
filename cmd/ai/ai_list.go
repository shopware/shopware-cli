package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/installer"
	"github.com/shopware/shopware-cli/internal/tui"
)

var aiListCmd = &cobra.Command{
	Use:          "list",
	Aliases:      []string{"ls"},
	Short:        "List available Shopware AI integrations",
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		format, err := resolveFormat(cmd)
		if err != nil {
			return err
		}

		installedOnly, _ := cmd.Flags().GetBool("installed")
		global, _ := cmd.Flags().GetBool("global")
		if global && !installedOnly {
			return errors.New("cannot use --global without --installed")
		}

		var installed map[string]bool
		if installedOnly {
			if installed, err = installer.InstalledNames(cmd.Context(), global, cmd.ErrOrStderr()); err != nil {
				return err
			}
		}

		entries := directory.Load().List(installed, installedOnly)

		if format == formatJSON {
			return writeListJSON(cmd.OutOrStdout(), entries)
		}

		return writeListTable(cmd.OutOrStdout(), entries)
	},
}

func writeListJSON(w io.Writer, entries []directory.Integration) error {
	if entries == nil {
		entries = []directory.Integration{}
	}
	out, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, string(out))

	return err
}

func writeListTable(w io.Writer, entries []directory.Integration) error {
	rows := make([][]string, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, []string{e.Name, e.Description, e.Repository})
	}

	_, err := fmt.Fprintln(w, tui.RenderTable(
		[]string{"Name", "Description", "Repository"},
		rows,
	))

	return err
}

func init() {
	aiRootCmd.AddCommand(aiListCmd)
	aiListCmd.Flags().Bool("installed", false, "Show only installed skills reported by the skills CLI")
	aiListCmd.Flags().Bool("global", false, "Use the user directory instead of the current project")
	addFormatFlag(aiListCmd)
}

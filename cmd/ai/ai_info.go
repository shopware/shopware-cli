package ai

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/tui"
)

var aiInfoCmd = &cobra.Command{
	Use:          "info name",
	Short:        "Show details of a Shopware AI integration",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveFormat(cmd)
		if err != nil {
			return err
		}

		entry, err := directory.Load().Info(args[0])
		if err != nil {
			return err
		}

		result := tui.NewTable(
			tui.TableColumn{Title: "Name", JSONKey: "name"},
			tui.TableColumn{Title: "Description", JSONKey: "description"},
			tui.TableColumn{Title: "Repository", JSONKey: "repository"},
		)
		result.AddRow(entry.Name, entry.Description, entry.Repository)
		return result.Write(cmd.OutOrStdout(), tui.TableFormat(format))
	},
}

func init() {
	aiRootCmd.AddCommand(aiInfoCmd)
	addFormatFlag(aiInfoCmd)
}

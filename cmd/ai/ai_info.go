package ai

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/tui"
)

var aiInfoCmd = &cobra.Command{
	Use:          "info name",
	Short:        "Show details of a Shopware AI integration",
	Long:         `Show the details of a Shopware AI integration from the directory.`,
	Example:      `  shopware-cli ai info deployment-helper`,
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

		if format == formatJSON {
			return writeInfoJSON(cmd.OutOrStdout(), *entry)
		}

		return writeInfoTable(cmd.OutOrStdout(), *entry)
	},
}

func writeInfoJSON(w io.Writer, e directory.Integration) error {
	out, err := json.Marshal(e)
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(w, string(out))

	return err
}

// writeInfoTable prints an entry as a two-column property/value table.
func writeInfoTable(w io.Writer, e directory.Integration) error {
	rows := [][]string{
		{"Name", e.Name},
		{"Display name", e.DisplayName},
		{"Type", e.Type.String()},
		{"Provider", e.Provider},
		{"Status", string(e.Status)},
		{"Description", e.Description},
		{"Documentation", e.Documentation},
		{"Delivery", deliveryLabel(e.Delivery)},
	}

	_, err := fmt.Fprintln(w, tui.RenderTable([]string{"Property", "Value"}, rows))

	return err
}

// deliveryLabel renders a delivery block for the human view.
func deliveryLabel(d directory.Delivery) string {
	switch {
	case d.URL != "":
		if d.Transport != "" {
			return fmt.Sprintf("%s (%s, %s)", d.Kind, d.Transport, d.URL)
		}

		return fmt.Sprintf("%s (%s)", d.Kind, d.URL)
	case d.Repository != "":
		return fmt.Sprintf("%s (%s)", d.Kind, d.Repository)
	default:
		return string(d.Kind)
	}
}

func init() {
	aiRootCmd.AddCommand(aiInfoCmd)
	addFormatFlag(aiInfoCmd)
}

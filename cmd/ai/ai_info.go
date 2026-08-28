package ai

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
)

var aiInfoCmd = &cobra.Command{
	Use:          "info <name>",
	Short:        "Show details of a Shopware AI integration",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")

		entry, err := directory.Load().Info(args[0])
		if err != nil {
			return err
		}

		if asJSON {
			return writeInfoJSON(cmd.OutOrStdout(), entry)
		}

		return writeInfoTable(cmd.OutOrStdout(), entry)
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

// writeInfoTable prints an entry as aligned key/value lines for humans.
func writeInfoTable(w io.Writer, e directory.Integration) error {
	lines := [][2]string{
		{"Name", e.Name},
		{"Display name", e.DisplayName},
		{"Type", string(e.Type)},
		{"Provider", e.Provider},
		{"Status", string(e.Status)},
		{"Description", e.Description},
		{"Documentation", e.Documentation},
		{"Delivery", deliveryLabel(e.Delivery)},
		{"Compatibility", compatibilityLabel(e)},
	}

	for _, l := range lines {
		if _, err := fmt.Fprintf(w, "%-14s %s\n", l[0]+":", l[1]); err != nil {
			return err
		}
	}

	return nil
}

// compatibilityLabel renders the compatibility requirements for the human view.
// An entry without a compatibility block has no requirements; a bundled entry
// ships with the CLI and is therefore always compatible.
func compatibilityLabel(e directory.Integration) string {
	if e.Compatibility != nil {
		return e.Compatibility.Source
	}
	if e.Delivery.Kind == directory.DeliveryBundled {
		return "none (bundled, always compatible)"
	}

	return "none"
}

// deliveryLabel renders a delivery block for the human view.
func deliveryLabel(d directory.Delivery) string {
	if d.Repository != "" {
		return fmt.Sprintf("%s (%s)", d.Kind, d.Repository)
	}

	return string(d.Kind)
}

func init() {
	aiRootCmd.AddCommand(aiInfoCmd)
	aiInfoCmd.Flags().Bool("json", false, "Output as json")
}

package ai

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
)

var aiInfoCmd = newAIInfoCmd()

func newAIInfoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "info <name>",
		Short:        "Show details of a Shopware AI integration",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE:         runAIInfo,
	}

	cmd.Flags().Bool("json", false, "Output as json")

	return cmd
}

func runAIInfo(cmd *cobra.Command, args []string) error {
	name := args[0]

	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	dir, err := directory.Load()
	if err != nil {
		return err
	}

	found, ok := dir.Get(name)
	if !ok {
		return fmt.Errorf("unknown integration %q", name)
	}

	entry := applyAvailability(*found)

	if asJSON {
		out, err := json.Marshal(entry)
		if err != nil {
			return err
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), string(out))

		return err
	}

	return writeInfoTable(cmd, entry)
}

// writeInfoTable prints an entry as aligned key/value lines for humans.
func writeInfoTable(cmd *cobra.Command, e directory.Integration) error {
	out := cmd.OutOrStdout()

	lines := [][2]string{
		{"Name", e.Name},
		{"Display name", e.DisplayName},
		{"Type", string(e.Type)},
		{"Provider", e.Provider},
		{"Status", string(e.Status)},
		{"Available", availableLabel(e)},
		{"Description", e.Description},
		{"Documentation", e.Documentation},
		{"Delivery", deliveryLabel(e.Delivery)},
	}
	if e.Compatibility != nil {
		lines = append(lines, [2]string{"Compatibility", e.Compatibility.Source})
	}

	for _, l := range lines {
		if _, err := fmt.Fprintf(out, "%-14s %s\n", l[0]+":", l[1]); err != nil {
			return err
		}
	}

	return nil
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
}

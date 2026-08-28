package ai

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	formatTable = "table"
	formatJSON  = "json"
)

// addFormatFlag registers the shared --format flag (see shopware/shopware-cli#1471).
func addFormatFlag(cmd *cobra.Command) {
	cmd.Flags().String("format", formatTable, "Output format (table, json)")
}

// resolveFormat reads and validates the --format flag.
func resolveFormat(cmd *cobra.Command) (string, error) {
	format, _ := cmd.Flags().GetString("format")

	switch format {
	case formatTable, formatJSON:
		return format, nil
	default:
		return "", fmt.Errorf("unknown --format %q (allowed: table, json)", format)
	}
}

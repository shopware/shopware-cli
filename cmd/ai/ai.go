package ai

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
)

var aiRootCmd = &cobra.Command{
	Use:   "ai",
	Short: "Discover Shopware AI integrations",
	// Hidden until the feature is complete (install/MCP land in #1336/#1337).
	// The commands still work when invoked; they are only kept out of --help so
	// incremental merges do not advertise a half-finished command group.
	Hidden: true,
}

// Register adds the ai command group to the root command.
func Register(rootCmd *cobra.Command) {
	rootCmd.AddCommand(aiRootCmd)
}

// applyAvailability fills the computed availability fields for an entry. v1 is
// static: a coming-soon entry is unavailable, everything else is available.
// Project-detected availability (Core MCP) arrives with #1336; there is no
// network access here.
func applyAvailability(e directory.Integration) directory.Integration {
	if e.Status == directory.StatusComingSoon {
		e.Available = false
		e.AvailabilityReason = "not yet released"

		return e
	}

	e.Available = true

	return e
}

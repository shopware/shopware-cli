package ai

import (
	"github.com/spf13/cobra"
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

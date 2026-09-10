package ai

import (
	"github.com/spf13/cobra"
)

var aiRootCmd = &cobra.Command{
	Use:   "ai",
	Short: "Discover Shopware AI integrations",
}

// Register adds the ai command group to the root command.
func Register(rootCmd *cobra.Command) {
	rootCmd.AddCommand(aiRootCmd)
}

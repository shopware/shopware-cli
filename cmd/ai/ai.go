package ai

import (
	"github.com/spf13/cobra"
)

var aiRootCmd = &cobra.Command{
	Use:   "ai",
	Short: "Discover Shopware AI integrations",
	Long: `Discover and install Shopware AI integrations (skills) into AI agents.

Integrations are installed through skills.sh, which requires Node.js/npx on PATH
(git-delivered integrations also need git and network access).`,
}

// Register adds the ai command group to the root command.
func Register(rootCmd *cobra.Command) {
	rootCmd.AddCommand(aiRootCmd)
}

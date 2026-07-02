package hub

import (
	"github.com/spf13/cobra"
)

var hubRootCmd = &cobra.Command{
	Use:   "hub",
	Short: "Shopware Community Hub commands",
}

// Register adds the hub command tree to rootCmd.
func Register(rootCmd *cobra.Command) {
	rootCmd.AddCommand(hubRootCmd)
}

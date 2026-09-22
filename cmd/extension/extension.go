package extension

import (
	"github.com/spf13/cobra"
)

var extensionRootCmd = &cobra.Command{
	Use:   "extension",
	Short: "Build, validate, package, and maintain Shopware extensions",
}

func Register(rootCmd *cobra.Command) {
	rootCmd.AddCommand(extensionRootCmd)
}

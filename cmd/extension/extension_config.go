package extension

import (
	"github.com/spf13/cobra"
)

var extensionConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the extension config",
}

func init() {
	extensionRootCmd.AddCommand(extensionConfigCmd)
}

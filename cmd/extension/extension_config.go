package extension

import (
	"github.com/spf13/cobra"
)

var extensionConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the extension configuration file",
}

func init() {
	extensionRootCmd.AddCommand(extensionConfigCmd)
}

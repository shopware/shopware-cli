package extension

import (
	"github.com/spf13/cobra"
)

var extensionConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Create the extension configuration file",
}

func init() {
	extensionRootCmd.AddCommand(extensionConfigCmd)
}

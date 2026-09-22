package extension

import (
	"github.com/spf13/cobra"
)

var extensionConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Create extension config or print its schema",
}

func init() {
	extensionRootCmd.AddCommand(extensionConfigCmd)
}

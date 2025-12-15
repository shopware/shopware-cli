package extension

import "github.com/spf13/cobra"

var extensionAiCmd = &cobra.Command{
	Use:   "ai",
	Short: "AI commands (experimental)",
}

func init() {
	extensionRootCmd.AddCommand(extensionAiCmd)
}

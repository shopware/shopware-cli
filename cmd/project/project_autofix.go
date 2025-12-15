package project

import "github.com/spf13/cobra"

var projectAutofixCmd = &cobra.Command{
	Use:   "autofix",
	Short: "Autofix a project",
}

func init() {
	projectRootCmd.AddCommand(projectAutofixCmd)
}

package project

import "github.com/spf13/cobra"

var projectAutofixCmd = &cobra.Command{
	Use:   "autofix",
	Short: "Migrate a project to Composer or Symfony Flex",
}

func init() {
	projectRootCmd.AddCommand(projectAutofixCmd)
}

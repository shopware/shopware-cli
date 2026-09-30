package project

import (
	"github.com/spf13/cobra"
)

var projectConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage the project configuration file",
}

func init() {
	projectRootCmd.AddCommand(projectConfigCmd)
}

package project

import (
	"github.com/spf13/cobra"
)

var projectConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Create or inspect project configuration",
}

func init() {
	projectRootCmd.AddCommand(projectConfigCmd)
}

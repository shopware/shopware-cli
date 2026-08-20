package project

import (
	"github.com/spf13/cobra"
)

var projectProxyUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Register the current project with the shared proxy and start it",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := newProxyEnvironment(cmd)
		if err != nil {
			return err
		}

		return env.up(cmd)
	},
}

func init() {
	projectProxyCmd.AddCommand(projectProxyUpCmd)
}

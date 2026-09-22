package project

import (
	"github.com/spf13/cobra"
)

var projectProxyUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Route the current project through the shared proxy",
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

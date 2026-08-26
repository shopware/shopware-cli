package project

import (
	"github.com/spf13/cobra"
)

var projectProxyDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Deregister the current project from the shared proxy and stop it",
	RunE: func(cmd *cobra.Command, args []string) error {
		env, err := newProxyEnvironment(cmd)
		if err != nil {
			return err
		}

		return env.down(cmd.Context(), true)
	},
}

func init() {
	projectProxyCmd.AddCommand(projectProxyDownCmd)
}

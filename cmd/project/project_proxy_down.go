package project

import (
	"github.com/spf13/cobra"
)

var projectProxyDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop routing and restore the current project's URL",
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

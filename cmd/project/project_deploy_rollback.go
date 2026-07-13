package project

import (
	"github.com/spf13/cobra"
)

var projectDeployRollbackCmd = &cobra.Command{
	Use:   "rollback [release]",
	Short: "Roll back to a previous release",
	Long: `Switches the current symlink back to a previous release.

Without an argument the release deployed before the currently active one is used.
The release that was rolled back from is marked as bad and skipped by later rollbacks.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var release string
		if len(args) > 0 {
			release = args[0]
		}

		deployer, cleanup, err := resolveDeployer(cmd)
		if err != nil {
			return err
		}
		defer cleanup()

		return deployer.Rollback(cmd.Context(), release)
	},
}

func init() {
	projectDeployCmd.AddCommand(projectDeployRollbackCmd)
}

package project

import (
	"fmt"

	"github.com/spf13/cobra"
)

var projectDeployReleasesCmd = &cobra.Command{
	Use:   "releases",
	Short: "List the releases on the deployment target",
	RunE: func(cmd *cobra.Command, _ []string) error {
		deployer, cleanup, err := resolveDeployer(cmd)
		if err != nil {
			return err
		}
		defer cleanup()

		releases, err := deployer.Releases(cmd.Context())
		if err != nil {
			return err
		}

		if len(releases) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No releases found")
			return nil
		}

		for _, release := range releases {
			marker := ""
			if release.Active {
				marker = " (active)"
			}
			if release.Bad {
				marker += " (bad)"
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", release.Name, marker)
		}

		return nil
	},
}

func init() {
	projectDeployCmd.AddCommand(projectDeployReleasesCmd)
}

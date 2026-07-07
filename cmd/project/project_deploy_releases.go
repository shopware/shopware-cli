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

		hostReleases, err := deployer.Releases(cmd.Context())
		if err != nil {
			return err
		}

		for i, hr := range hostReleases {
			if len(hostReleases) > 1 {
				if i > 0 {
					fmt.Fprintln(cmd.OutOrStdout())
				}

				fmt.Fprintf(cmd.OutOrStdout(), "Host %s:\n", hr.Host)
			}

			if len(hr.Releases) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No releases found")
				continue
			}

			for _, release := range hr.Releases {
				marker := ""
				if release.Active {
					marker = " (active)"
				}
				if release.Bad {
					marker += " (bad)"
				}

				fmt.Fprintf(cmd.OutOrStdout(), "%s%s\n", release.Name, marker)
			}
		}

		return nil
	},
}

func init() {
	projectDeployCmd.AddCommand(projectDeployReleasesCmd)
}

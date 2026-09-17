//go:build deployment

package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
)

var projectDeploymentPackageCmd = &cobra.Command{
	Use:   "package",
	Short: "Build deployable project artifacts without rolling them out",
}

func init() {
	projectDeploymentCmd.AddCommand(projectDeploymentPackageCmd)
}

func packageProjectConfigPath(cmd *cobra.Command, root string) string {
	if cmd.Flags().Changed("project-config") {
		return projectConfigPath
	}
	return shop.SearchConfigPath(cmd.Context(), root, projectConfigPath)
}

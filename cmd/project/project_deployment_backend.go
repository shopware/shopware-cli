//go:build deployment

package project

import (
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	deploymentbackend "github.com/shopware/shopware-cli/internal/deployment/backend"
)

func resolveProjectDeploymentBackend(cmd *cobra.Command, root string) (deployment.Backend, error) {
	return deploymentbackend.ResolveBackend(cmd.Context(), root, projectConfigPath, environmentName)
}

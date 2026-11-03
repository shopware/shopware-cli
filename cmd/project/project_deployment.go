//go:build deployment

package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	deploymentbackend "github.com/shopware/shopware-cli/internal/deployment/backend"
	"github.com/shopware/shopware-cli/internal/shop"
)

var projectDeploymentCmd = &cobra.Command{
	Use:     "deploy",
	Aliases: []string{"deployment"},
	Short:   "Manage project deployments",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if environmentName != "" {
			return nil
		}
		// Only these commands accept a project directory; other positional
		// arguments identify deployments, not paths.
		var projectArgs []string
		switch cmd {
		case projectDeploymentInitCmd, projectDeploymentCreateCmd, projectDeploymentPackageArchiveCmd, projectDeploymentPackageContainerCmd:
			projectArgs = args
		}
		root, err := resolveDeploymentProjectRoot(projectArgs)
		if err != nil {
			return err
		}
		name, err := deploymentbackend.SelectEnvironment(cmd.Context(), root, projectConfigPath, cmd.InOrStdin(), cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		return cmd.Flags().Set("env", name)
	},
}

func init() {
	projectRootCmd.AddCommand(projectDeploymentCmd)
}

func resolveDeploymentProjectRoot(args []string) (string, error) {
	var root string
	var err error
	if len(args) == 1 {
		root, err = filepath.Abs(args[0])
	} else {
		root, err = shop.FindClosestShopwareProject(false)
	}
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil {
		return "", fmt.Errorf("cannot read project directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("cannot use project path %q: it is not a directory", root)
	}
	return root, nil
}

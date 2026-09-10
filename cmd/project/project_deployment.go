//go:build deployment

package project

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
)

var projectDeploymentCmd = &cobra.Command{
	Use:   "deployment",
	Short: "Manage project deployments",
	Long:  "Create immutable project deployments separately from rolling them out to an environment.",
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
		return "", fmt.Errorf("read project directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("project path %q is not a directory", root)
	}
	return root, nil
}

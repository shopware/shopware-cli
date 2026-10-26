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
	Use:     "deploy",
	Aliases: []string{"deployment"},
	Short:   "Manage project deployment artifacts",
	Long:    "Create immutable deployments and package projects into archives or container build files.",
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

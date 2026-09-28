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
	Use:   "deploy",
	Short: "Manage project deployments",
	Long: `Initialize deployment environments and create immutable deployments separately from rolling them out.

For multiple SSH hosts, rollout and rollback target the entire group. Persistent
shared paths must already be provisioned consistently across hosts. Deployment
Helper runs once on migration_host; each host prepares and activates its own release.
Activation is sequential, not atomic across hosts; a failure stops further switches.

Use --ssh-host for single-host init, prune, logs or list. Init and prune require
an explicit host in a group; logs otherwise use migration_host.`,
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

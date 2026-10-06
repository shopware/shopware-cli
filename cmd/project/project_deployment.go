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

Init prompts once on migration_host and verifies shared runtime configuration on
all hosts. Prune applies retention independently to each host and reports partial
failures. List shows all hosts; logs use migration_host.`,
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

package project

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/shop"
)

var projectDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy the project to a remote environment",
	Long: `Deploys the project to the environment selected with --env.

The project is uploaded as a new release into <deployment.path>/releases, shared
files and directories are linked from <deployment.path>/shared and the
<deployment.path>/current symlink is switched atomically to the new release.
Use "shopware-cli project deploy rollback" to switch back to a previous release.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		skipBuildHooks, _ := cmd.Flags().GetBool("skip-build-hooks")

		deployer, cleanup, err := resolveDeployer(cmd)
		if err != nil {
			return err
		}
		defer cleanup()

		return deployer.Deploy(cmd.Context(), deployment.Options{SkipBuildHooks: skipBuildHooks})
	},
}

// resolveDeployer creates the Deployer for the selected environment.
func resolveDeployer(cmd *cobra.Command) (deployment.Deployer, func(), error) {
	projectRoot, err := findClosestShopwareProject()
	if err != nil {
		return nil, nil, err
	}

	cfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, false)
	if err != nil {
		return nil, nil, err
	}

	if environmentName == "" {
		return nil, nil, fmt.Errorf("no environment selected, use --env to pick one of the environments defined in %s", projectConfigPath)
	}

	envCfg, err := cfg.ResolveEnvironment(environmentName)
	if err != nil {
		return nil, nil, err
	}

	deployer, err := deployment.NewDeployer(projectRoot, envCfg, cfg)
	if err != nil {
		return nil, nil, err
	}

	return deployer, func() { _ = deployer.Close() }, nil
}

func init() {
	projectDeployCmd.Flags().Bool("skip-build-hooks", false, "Skip the local build hooks before the upload")
	projectRootCmd.AddCommand(projectDeployCmd)
}

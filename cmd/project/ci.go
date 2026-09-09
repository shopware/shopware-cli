package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	internalgit "github.com/shopware/shopware-cli/internal/git"
	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var projectCI = &cobra.Command{
	Use:   "ci",
	Short: "Build Shopware in the CI",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return err
		}
		if err := projectCISafetyCheck(cmd.Context(), root, force, os.Getenv); err != nil {
			return err
		}

		shopCfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}
		envCfg, err := shopCfg.ResolveEnvironment(environmentName)
		if err != nil {
			return err
		}
		withDev, err := cmd.Flags().GetBool("with-dev-dependencies")
		if err != nil {
			return err
		}

		return projectbuild.Build(cmd.Context(), root, shopCfg, envCfg, projectbuild.Options{
			WithDevDependencies: withDev,
			ToolVersion:         tui.AppVersion,
		})
	},
}

func init() {
	projectRootCmd.AddCommand(projectCI)
	projectCI.PersistentFlags().Bool("with-dev-dependencies", false, "Install dev dependencies")
	projectCI.PersistentFlags().Bool("force", false, "Run project ci outside CI even when the git working tree has local changes")
}

func projectCISafetyCheck(ctx context.Context, root string, force bool, getenv func(string) string) error {
	if force || system.IsCIEnvironment(getenv) {
		return nil
	}

	dirty, isGitRepository, err := internalgit.IsWorkingTreeDirty(ctx, root)
	if err != nil {
		return err
	}

	if !isGitRepository {
		logging.FromContext(ctx).Warnf("Running project ci outside a CI environment; this command removes source files and should usually only be used in CI")
		return nil
	}

	if dirty {
		return errors.New("project ci removes source files and creates build stubs; refusing to run outside CI with a dirty git working tree. Commit, stash, or clean local changes, or pass --force if you intentionally want to run it")
	}

	logging.FromContext(ctx).Warnf("Running project ci outside a CI environment; this command removes source files and should usually only be used in CI")

	return nil
}

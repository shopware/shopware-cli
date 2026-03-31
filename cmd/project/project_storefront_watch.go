package project

import (
	"context"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
)

var projectStorefrontWatchCmd = &cobra.Command{
	Use:     "storefront-watch [path]",
	Short:   "Starts the Shopware Storefront Watcher",
	Aliases: []string{"watch-storefront"},
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectRoot string
		var err error

		if len(args) == 1 {
			projectRoot = args[0]
		} else if projectRoot, err = findClosestShopwareProject(); err != nil {
			return err
		}

		if err := envfile.LoadSymfonyEnvFile(projectRoot); err != nil {
			return err
		}

		shopCfg, err := shop.ReadConfig(cmd.Context(), projectConfigPath, true)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		if err := filterAndWritePluginJson(cmd, projectRoot, shopCfg, cmdExecutor); err != nil {
			return err
		}

		watchProcess, err := extension.PrepareStorefrontWatcher(cmd.Context(), projectRoot, cmdExecutor)
		if err != nil {
			return err
		}

		runErr := runTransparentCommand(watchProcess)

		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = watchProcess.Stop(stopCtx)

		return runErr
	},
}

func init() {
	projectRootCmd.AddCommand(projectStorefrontWatchCmd)
	projectStorefrontWatchCmd.PersistentFlags().String("only-extensions", "", "Only watch the given extensions (comma separated)")
	projectStorefrontWatchCmd.PersistentFlags().String("skip-extensions", "", "Skips the given extensions (comma separated)")
	projectStorefrontWatchCmd.PersistentFlags().Bool("only-custom-static-extensions", false, "Only build extensions from custom/static-plugins directory")
}

package project

import (
	"context"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/projectbuild"
	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectStorefrontWatchCmd = &cobra.Command{
	Use:     "storefront-watch [path]",
	Short:   "Watch Storefront assets and rebuild on change",
	Aliases: []string{"watch-storefront"},
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var projectRoot string
		var err error

		if len(args) == 1 {
			projectRoot = args[0]
		} else if projectRoot, err = shop.FindClosestShopwareProject(false); err != nil {
			return err
		}

		if err := envfile.LoadSymfonyEnvFile(projectRoot); err != nil {
			return err
		}

		actualProjectConfigPath := shop.SearchConfigPath(cmd.Context(), projectRoot, projectConfigPath)
		shopCfg, err := shop.ReadConfig(cmd.Context(), actualProjectConfigPath, true)
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

		var opts extension.StorefrontWatcherOptions
		if cmd.PersistentFlags().Changed("sales-channel") {
			salesChannelID, _ := cmd.PersistentFlags().GetString("sales-channel")
			opts, err = extension.ResolveStorefrontWatcherOptions(cmd.Context(), cmdExecutor, salesChannelID)
			if err != nil {
				return err
			}
		}

		// When the shop is proxied, route the webpack hot-proxy watcher through
		// the shared proxy at its storefront-watch hostname. An unreadable
		// registry must not silently start the watcher unproxied.
		host, err := proxy.RegisteredHostname(projectRoot)
		if err != nil {
			logging.FromContext(cmd.Context()).Errorf("Could not read the shared proxy registry: %v", err)
			return err
		}
		if host != "" {
			opts.ProxyHostname = "storefront-watch." + host
		}

		watchProcess, err := extension.PrepareStorefrontWatcher(cmd.Context(), projectRoot, cmdExecutor, opts, cmd.InOrStdin(), os.Stdout)
		if err != nil {
			return err
		}

		watchProcess.Cmd.Stdin = cmd.InOrStdin()
		runErr := projectbuild.RunCommand(watchProcess)

		stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer stopCancel()
		_ = watchProcess.Stop(stopCtx)

		return runErr
	},
}

func init() {
	projectRootCmd.AddCommand(projectStorefrontWatchCmd)
	projectStorefrontWatchCmd.PersistentFlags().String("only-extensions", "", "Watch only the specified extensions (comma-separated)")
	projectStorefrontWatchCmd.PersistentFlags().Bool("select-extensions", false, "Select extensions interactively")
	projectStorefrontWatchCmd.PersistentFlags().String("skip-extensions", "", "Skip the specified extensions (comma-separated)")
	projectStorefrontWatchCmd.PersistentFlags().Bool("only-custom-static-extensions", false, "Watch only extensions in the custom/static-plugins directory")
	projectStorefrontWatchCmd.PersistentFlags().String("sales-channel", "", "Sales channel ID to target with theme:dump. Pass without a value (--sales-channel) to pick interactively. Omit the flag entirely to keep the legacy theme:dump behavior")
	projectStorefrontWatchCmd.PersistentFlags().Lookup("sales-channel").NoOptDefVal = " "
}

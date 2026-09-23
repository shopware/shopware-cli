package project

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
	"github.com/shopwareLabs/go-shopware-http-client/instance"
)

var projectClearCacheCmd = &cobra.Command{
	Use:   "clear-cache",
	Short: "Clear a Shopware project's cache locally or via Admin API",
	RunE: func(cmd *cobra.Command, _ []string) error {
		projectRoot, err := shop.FindClosestShopwareProject(true)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		cfg := cmdExecutor.ShopConfig()
		if cfg == nil || cfg.AdminApi == nil {
			projectRoot, err = shop.FindClosestShopwareProject(false)
			if err != nil {
				return err
			}

			logging.FromContext(cmd.Context()).Infof("Clearing cache locally")

			return os.RemoveAll(projectRoot + "/var/cache")
		}

		logging.FromContext(cmd.Context()).Infof("Clearing cache using admin-api")

		client, err := cmdExecutor.AdminAPIClient(cmd.Context())
		if err != nil {
			return err
		}

		return instance.NewManager(client).ClearCache(cmd.Context())
	},
}

func init() {
	projectRootCmd.AddCommand(projectClearCacheCmd)
}

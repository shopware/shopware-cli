package project

import (
	"os"

	adminSdk "github.com/friendsofshopware/go-shopware-admin-api-sdk"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
)

var projectConfigPullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Synchronizes your shop config to local",
	RunE: func(cmd *cobra.Command, _ []string) error {
		var cfg *shop.Config
		var err error

		if cfg, err = shop.ReadConfig(projectConfigPath, false); err != nil {
			return err
		}

		client, err := shop.NewShopClient(cmd.Context(), cfg)
		if err != nil {
			return err
		}

		for _, applyer := range NewSyncApplyers(cfg) {
			if err := applyer.Pull(adminSdk.NewApiContext(cmd.Context()), client, cfg); err != nil {
				return err
			}
		}

		content, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}

		if err := os.WriteFile(projectConfigPath, content, os.ModePerm); err != nil {
			return err
		}

		logging.FromContext(cmd.Context()).Infof("%s has been updated", projectConfigPath)

		return nil
	},
}

func init() {
	projectConfigCmd.AddCommand(projectConfigPullCmd)
}

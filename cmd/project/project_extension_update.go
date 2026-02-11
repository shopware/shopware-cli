package project

import (
	"fmt"

	"github.com/spf13/cobra"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectExtensionUpdateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Update a extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var cfg *shop.Config
		var err error

		if cfg, err = shop.ReadConfig(cmd.Context(), projectConfigPath, true); err != nil {
			return err
		}

		client, err := shop.NewShopClient(cmd.Context(), cfg)
		if err != nil {
			return err
		}

		disableStoreUpdates, _ := cmd.PersistentFlags().GetBool("disable-store-update")

		if _, err := client.ExtensionManager.Refresh(adminSdk.NewApiContext(cmd.Context())); err != nil {
			return err
		}

		extensions, _, err := client.ExtensionManager.ListAvailableExtensions(adminSdk.NewApiContext(cmd.Context()))
		if err != nil {
			return err
		}

		failed := false

		if len(args) == 1 && args[0] == "all" {
			args = make([]string, 0)

			for _, extension := range extensions {
				args = append(args, extension.Name)
			}
		}

		for _, arg := range args {
			extension := extensions.GetByName(arg)

			if extension == nil {
				failed = true
				logging.FromContext(cmd.Context()).Errorf("Cannot find extension by name %s", arg)
				continue
			}

			if !extension.IsUpdateAble() {
				logging.FromContext(cmd.Context()).Infof("Extension %s is up to date", arg)
				continue
			}

			if !extension.Active {
				logging.FromContext(cmd.Context()).Infof("Extension %s is not active skipping", arg)
				continue
			}

			if extension.UpdateSource == "store" && !disableStoreUpdates {
				if _, err := client.ExtensionManager.DownloadExtension(adminSdk.NewApiContext(cmd.Context()), arg); err != nil {
					logging.FromContext(cmd.Context()).Errorf("Download of %s update failed with error: %v", extension.Name, err)
					failed = true
					continue
				}
			}

			if _, err := client.ExtensionManager.UpdateExtension(adminSdk.NewApiContext(cmd.Context()), extension.Type, extension.Name); err != nil {
				failed = true

				logging.FromContext(cmd.Context()).Errorf("Update of %s failed with error: %v", extension.Name, err)
			}

			logging.FromContext(cmd.Context()).Infof("Updated %s", extension.Name)
		}

		if failed {
			return fmt.Errorf("update failed")
		}

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionUpdateCmd)
	projectExtensionUpdateCmd.PersistentFlags().Bool("disable-store-update", false, "Downloads updates from store.shopware.com")
}

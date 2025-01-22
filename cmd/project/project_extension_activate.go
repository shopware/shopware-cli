package project

import (
	"fmt"

	adminSdk "github.com/friendsofshopware/go-shopware-admin-api-sdk"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
)

var projectExtensionActivateCmd = &cobra.Command{
	Use:   "activate [name]",
	Short: "Activate a extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var cfg *shop.Config
		var err error

		if cfg, err = shop.ReadConfig(projectConfigPath, false); err != nil {
			return err
		}

		client, err := shop.NewShopClient(cmd.Context(), cfg)
		if err != nil {
			return err
		}

		extensions, _, err := client.ExtensionManager.ListAvailableExtensions(adminSdk.NewApiContext(cmd.Context()))
		if err != nil {
			return err
		}

		failed := false

		for _, arg := range args {
			extension := extensions.GetByName(arg)

			if extension == nil {
				failed = true
				logging.FromContext(cmd.Context()).Errorf("Cannot find extension by name %s", arg)
				continue
			}

			if extension.Active {
				logging.FromContext(cmd.Context()).Infof("Extension %s is already active", arg)
				continue
			}

			if extension.InstalledAt == nil {
				if _, err := client.ExtensionManager.InstallExtension(adminSdk.NewApiContext(cmd.Context()), extension.Type, extension.Name); err != nil {
					failed = true

					logging.FromContext(cmd.Context()).Errorf("Installation of %s failed with error: %v", extension.Name, err)
				}
			}

			if _, err := client.ExtensionManager.ActivateExtension(adminSdk.NewApiContext(cmd.Context()), extension.Type, extension.Name); err != nil {
				failed = true

				logging.FromContext(cmd.Context()).Errorf("Activate of %s failed with error: %v", extension.Name, err)
			}

			logging.FromContext(cmd.Context()).Infof("Activated %s", extension.Name)
		}

		if failed {
			return fmt.Errorf("activation failed")
		}

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionActivateCmd)
}

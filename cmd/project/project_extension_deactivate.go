package project

import (
	"fmt"

	"github.com/spf13/cobra"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectExtensionDeactivateCmd = &cobra.Command{
	Use:   "deactivate [name]",
	Short: "Deactivate a extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var cfg *shop.Config
		var err error

		if cfg, err = shop.ReadConfig(projectConfigPath, true); err != nil {
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

			if !extension.Active {
				logging.FromContext(cmd.Context()).Infof("Extension %s is already deactivated", arg)
				continue
			}

			if _, err := client.ExtensionManager.DeactivateExtension(adminSdk.NewApiContext(cmd.Context()), extension.Type, extension.Name); err != nil {
				failed = true

				logging.FromContext(cmd.Context()).Errorf("Deactivation of %s failed with error: %v", extension.Name, err)
			}

			logging.FromContext(cmd.Context()).Infof("Deactivated %s", extension.Name)
		}

		if failed {
			return fmt.Errorf("deactivation failed")
		}

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionDeactivateCmd)
}

package project

import (
	"fmt"

	"github.com/spf13/cobra"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectExtensionInstallCmd = &cobra.Command{
	Use:   "install [name]",
	Short: "Install a extension",
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

		activateAfterInstall, _ := cmd.PersistentFlags().GetBool("activate")

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

			if extension.InstalledAt != nil {
				logging.FromContext(cmd.Context()).Infof("Extension %s is already installed", arg)
				continue
			}

			if _, err := client.ExtensionManager.InstallExtension(adminSdk.NewApiContext(cmd.Context()), extension.Type, extension.Name); err != nil {
				failed = true

				logging.FromContext(cmd.Context()).Errorf("Installation of %s failed with error: %v", extension.Name, err)
			}

			if activateAfterInstall {
				if _, err := client.ExtensionManager.ActivateExtension(adminSdk.NewApiContext(cmd.Context()), extension.Type, extension.Name); err != nil {
					failed = true

					logging.FromContext(cmd.Context()).Errorf("Activation of %s failed with error: %v", extension.Name, err)
				} else {
					logging.FromContext(cmd.Context()).Infof("Activated %s", extension.Name)
				}
			}

			logging.FromContext(cmd.Context()).Infof("Installed %s", extension.Name)
		}

		if failed {
			return fmt.Errorf("install failed")
		}

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionInstallCmd)
	projectExtensionInstallCmd.PersistentFlags().Bool("activate", false, "Activate the extension")
}

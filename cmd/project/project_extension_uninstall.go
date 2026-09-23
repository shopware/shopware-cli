package project

import (
	"errors"

	"github.com/shopwareLabs/go-shopware-http-client/extension"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectExtensionUninstallCmd = &cobra.Command{
	Use:   "uninstall name...",
	Short: "Uninstall an extension from a Shopware project",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := shop.FindClosestShopwareProject(true)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		client, err := cmdExecutor.AdminAPIClient(cmd.Context())
		if err != nil {
			return err
		}

		extensionManager := extension.NewManager(client)

		extensions, err := extensionManager.ListAvailable(cmd.Context())
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

			if extension.InstalledAt == nil {
				logging.FromContext(cmd.Context()).Infof("Extension %s is already uninstalled", arg)
				continue
			}

			if extension.Active {
				if err := extensionManager.Deactivate(cmd.Context(), extension.Type, extension.Name); err != nil {
					failed = true

					logging.FromContext(cmd.Context()).Errorf("Deactivation of %s failed with error: %v", extension.Name, err)
				} else {
					logging.FromContext(cmd.Context()).Infof("Deactivated %s", extension.Name)
				}
			}

			if err := extensionManager.Uninstall(cmd.Context(), extension.Type, extension.Name); err != nil {
				failed = true

				logging.FromContext(cmd.Context()).Errorf("Uninstall of %s failed with error: %v", extension.Name, err)
				continue
			}

			logging.FromContext(cmd.Context()).Infof("Uninstalled %s", extension.Name)
		}

		if failed {
			return errors.New("uninstall failed")
		}

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionUninstallCmd)
}

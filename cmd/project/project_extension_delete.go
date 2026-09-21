package project

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectExtensionDeleteCmd = &cobra.Command{
	Use:   "delete name...",
	Short: "Delete an extension from a Shopware project",
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

		extensions, err := client.ExtensionManager.ListAvailable(cmd.Context())
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
				if err := client.ExtensionManager.Deactivate(cmd.Context(), extension.Type, extension.Name); err != nil {
					failed = true

					logging.FromContext(cmd.Context()).Errorf("Deactivation of %s failed with error: %v", extension.Name, err)
					continue
				}

				logging.FromContext(cmd.Context()).Infof("Deactivated %s", extension.Name)
			}

			if extension.InstalledAt != nil {
				if err := client.ExtensionManager.Uninstall(cmd.Context(), extension.Type, extension.Name); err != nil {
					failed = true

					logging.FromContext(cmd.Context()).Errorf("Uninstall of %s failed with error: %v", extension.Name, err)
					continue
				}

				logging.FromContext(cmd.Context()).Infof("Uninstalled %s", extension.Name)
			}

			if err := client.ExtensionManager.Remove(cmd.Context(), extension.Type, extension.Name); err != nil {
				failed = true

				logging.FromContext(cmd.Context()).Errorf("Remove of %s failed with error: %v", extension.Name, err)
				continue
			}

			logging.FromContext(cmd.Context()).Infof("Removed %s", extension.Name)
		}

		if failed {
			return errors.New("remove failed")
		}

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionDeleteCmd)
}

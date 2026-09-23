package project

import (
	"errors"

	"github.com/shopwareLabs/go-shopware-http-client/extension"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

var projectExtensionUpdateCmd = &cobra.Command{
	Use:   "update name...|all",
	Short: "Update an installed extension",
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

		disableStoreUpdates, _ := cmd.PersistentFlags().GetBool("disable-store-update")

		if err := extensionManager.Refresh(cmd.Context()); err != nil {
			return err
		}

		extensions, err := extensionManager.ListAvailable(cmd.Context())
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

			if !extension.IsUpdatable() {
				logging.FromContext(cmd.Context()).Infof("Extension %s is up to date", arg)
				continue
			}

			if !extension.Active {
				logging.FromContext(cmd.Context()).Infof("Extension %s is not active skipping", arg)
				continue
			}

			if extension.UpdateSource == "store" && !disableStoreUpdates {
				if err := extensionManager.Download(cmd.Context(), arg); err != nil {
					logging.FromContext(cmd.Context()).Errorf("Download of %s update failed with error: %v", extension.Name, err)
					failed = true
					continue
				}
			}

			if err := extensionManager.Update(cmd.Context(), extension.Type, extension.Name); err != nil {
				failed = true

				logging.FromContext(cmd.Context()).Errorf("Update of %s failed with error: %v", extension.Name, err)
				continue
			}

			logging.FromContext(cmd.Context()).Infof("Updated %s", extension.Name)
		}

		if failed {
			return errors.New("update failed")
		}

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionUpdateCmd)
	projectExtensionUpdateCmd.PersistentFlags().Bool("disable-store-update", false, "Disable downloading updates from store.shopware.com")
}

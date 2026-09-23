package project

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
	"github.com/shopwareLabs/go-shopware-http-client/extension"
)

var projectExtensionActivateCmd = &cobra.Command{
	Use:   "activate name...",
	Short: "Activate an installed extension",
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

			if extension.Active {
				logging.FromContext(cmd.Context()).Infof("Extension %s is already active", arg)
				continue
			}

			if extension.InstalledAt == nil {
				if err := extensionManager.Install(cmd.Context(), extension.Type, extension.Name); err != nil {
					failed = true

					logging.FromContext(cmd.Context()).Errorf("Installation of %s failed with error: %v", extension.Name, err)
					continue
				}
			}

			if err := extensionManager.Activate(cmd.Context(), extension.Type, extension.Name); err != nil {
				failed = true

				logging.FromContext(cmd.Context()).Errorf("Activate of %s failed with error: %v", extension.Name, err)
				continue
			}

			logging.FromContext(cmd.Context()).Infof("Activated %s", extension.Name)
		}

		if failed {
			return errors.New("activation failed")
		}

		return nil
	},
}

func init() {
	projectExtensionCmd.AddCommand(projectExtensionActivateCmd)
}

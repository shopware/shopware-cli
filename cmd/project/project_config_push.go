package project

import (
	"encoding/json"

	adminSdk "github.com/friendsofshopware/go-shopware-admin-api-sdk"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
)

var projectConfigPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Synchronizes your local config to the external shop",
	RunE: func(cmd *cobra.Command, _ []string) error {
		logFormat := "Payload: %s"

		var cfg *shop.Config
		var err error

		apiCtx := adminSdk.NewApiContext(cmd.Context())

		autoApprove, _ := cmd.PersistentFlags().GetBool("auto-approve")

		if cfg, err = shop.ReadConfig(projectConfigPath, false); err != nil {
			return err
		}

		client, err := shop.NewShopClient(cmd.Context(), cfg)
		if err != nil {
			return err
		}

		operation := &ConfigSyncOperation{
			Operations:     map[string]adminSdk.SyncOperation{},
			SystemSettings: map[*string]map[string]interface{}{},
			ThemeSettings:  []ThemeSyncOperation{},
		}

		if cfg.Sync != nil {
			for _, applyer := range NewSyncApplyers(cfg) {
				if err := applyer.Push(apiCtx, client, cfg, operation); err != nil {
					return err
				}
			}
		}

		if !operation.HasChanges() {
			logging.FromContext(cmd.Context()).Infof("Configuration is up to date")
			return nil
		}

		if operation.Operations.HasChanges() {
			logging.FromContext(cmd.Context()).Infof("Following entities will be written")

			for _, values := range operation.Operations {
				logging.FromContext(cmd.Context()).Infof("Action: %s, Entity: %s", values.Action, values.Entity)

				content, _ := json.Marshal(values.Payload)

				logging.FromContext(cmd.Context()).Infof(logFormat, string(content))
			}
		}

		if operation.SystemSettings.HasChanges() {
			logging.FromContext(cmd.Context()).Infof("Following system_config changes will be applied")

			for key, values := range operation.SystemSettings {
				if len(values) == 0 {
					continue
				}

				var k string

				if key == nil {
					k = "default"
				} else {
					k = *key
				}

				logging.FromContext(cmd.Context()).Infof("Sales-Channel: %s", k)

				content, _ := json.Marshal(values)

				logging.FromContext(cmd.Context()).Infof(logFormat, string(content))
			}
		}

		if operation.ThemeSettings.HasChanges() {
			for _, themeOp := range operation.ThemeSettings {
				logging.FromContext(cmd.Context()).Infof("Updating theme: %s", themeOp.Name)

				content, _ := json.Marshal(themeOp.Settings)

				logging.FromContext(cmd.Context()).Infof(logFormat, string(content))
			}
		}

		if !autoApprove {
			p := promptui.Prompt{
				Label:     "You want to apply these changes to your Shop?",
				IsConfirm: true,
			}

			if _, err := p.Run(); err != nil {
				return err
			}
		}

		if _, err := client.Bulk.Sync(apiCtx, operation.Operations); err != nil {
			return err
		}

		if operation.SystemSettings.HasChanges() {
			if _, err := client.SystemConfigManager.UpdateConfig(apiCtx, operation.SystemSettings.ToJson()); err != nil {
				return err
			}
		}

		if operation.ThemeSettings.HasChanges() {
			for _, themeOp := range operation.ThemeSettings {
				if _, err := client.ThemeManager.UpdateConfiguration(apiCtx, themeOp.Id, adminSdk.ThemeUpdateRequest{Config: themeOp.Settings}); err != nil {
					return err
				}
			}
		}

		logging.FromContext(cmd.Context()).Infof("Configuration has been applied to remote")

		return nil
	},
}

func init() {
	projectConfigCmd.AddCommand(projectConfigPushCmd)
	projectConfigPushCmd.PersistentFlags().Bool("auto-approve", false, "Skips the confirmation")
}

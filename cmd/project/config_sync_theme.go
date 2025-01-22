package project

import (
	"bytes"
	"encoding/json"

	adminSdk "github.com/friendsofshopware/go-shopware-admin-api-sdk"

	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
)

type ThemeSync struct{}

func (ThemeSync) Push(ctx adminSdk.ApiContext, client *adminSdk.Client, config *shop.Config, operation *ConfigSyncOperation) error {
	if len(config.Sync.Theme) == 0 {
		return nil
	}

	criteria := adminSdk.Criteria{}
	criteria.Includes = map[string][]string{"theme": {"id", "name"}}
	themes, resp, err := client.Repository.Theme.SearchAll(ctx, criteria)
	if err != nil {
		return err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx.Context).Errorf("ThemeSync/Push: %v", err)
		}
	}()

	for _, t := range themes.Data {
		remoteConfigs, resp, err := client.ThemeManager.GetConfiguration(ctx, t.Id)
		if err != nil {
			return err
		}

		defer func() {
			if err := resp.Body.Close(); err != nil {
				logging.FromContext(ctx.Context).Errorf("ThemeSync/Push: %v", err)
			}
		}()

		for _, localThemeConfig := range config.Sync.Theme {
			if localThemeConfig.Name == t.Name {
				op := ThemeSyncOperation{
					Id:       t.Id,
					Name:     t.Name,
					Settings: map[string]adminSdk.ThemeConfigValue{},
				}

				for remoteFieldName, remoteFieldValue := range *remoteConfigs.CurrentFields {
					for localFieldName, localFieldValue := range localThemeConfig.Settings {
						if remoteFieldName == localFieldName {
							localJson, _ := json.Marshal(localFieldValue)
							remoteJson, _ := json.Marshal(remoteFieldValue)

							if !bytes.Equal(localJson, remoteJson) {
								op.Settings[remoteFieldName] = localFieldValue
							}
						}
					}
				}

				operation.ThemeSettings = append(operation.ThemeSettings, op)
			}
		}
	}

	return nil
}

func (ThemeSync) Pull(ctx adminSdk.ApiContext, client *adminSdk.Client, config *shop.Config) error {
	config.Sync.Theme = make([]shop.ThemeConfig, 0)

	criteria := adminSdk.Criteria{}
	criteria.Includes = map[string][]string{"theme": {"id", "name"}}
	themes, resp, err := client.Repository.Theme.SearchAll(ctx, criteria)
	if err != nil {
		return err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx.Context).Errorf("ThemeSync/Pull: %v", err)
		}
	}()

	for _, t := range themes.Data {
		cfg := shop.ThemeConfig{
			Name:     t.Name,
			Settings: map[string]adminSdk.ThemeConfigValue{},
		}

		themeConfig, resp, err := client.ThemeManager.GetConfiguration(ctx, t.Id)
		if err != nil {
			return err
		}

		defer func() {
			if err := resp.Body.Close(); err != nil {
				logging.FromContext(ctx.Context).Errorf("ThemeSync/Pull: %v", err)
			}
		}()

		cfg.Settings = *themeConfig.CurrentFields
		config.Sync.Theme = append(config.Sync.Theme, cfg)
	}

	return nil
}

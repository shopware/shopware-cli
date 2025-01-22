package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/shop"
	"github.com/spf13/cobra"
)

func findClosestShopwareProject() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		files := []string{
			fmt.Sprintf("%s/composer.json", currentDir),
			fmt.Sprintf("%s/composer.lock", currentDir),
		}

		for _, file := range files {
			if _, err := os.Stat(file); err == nil {
				content, err := os.ReadFile(file)
				if err != nil {
					return "", err
				}
				contentString := string(content)

				if strings.Contains(contentString, "shopware/core") {
					if _, err := os.Stat(fmt.Sprintf("%s/bin/console", currentDir)); err == nil {
						return currentDir, nil
					}
				}
			}
		}

		currentDir = filepath.Dir(currentDir)

		if currentDir == filepath.Dir(currentDir) {
			break
		}
	}

	return "", fmt.Errorf("cannot find Shopware project in current directory")
}

func filterAndWritePluginJson(cmd *cobra.Command, projectRoot string, shopCfg *shop.Config) error {
	sources, err := extension.DumpAndLoadAssetSourcesOfProject(cmd.Context(), projectRoot, shopCfg)
	if err != nil {
		return err
	}

	cfgs := extension.BuildAssetConfigFromExtensions(cmd.Context(), sources, extension.AssetBuildConfig{})

	onlyExtensions, _ := cmd.PersistentFlags().GetString("only-extensions")
	skipExtensions, _ := cmd.PersistentFlags().GetString("skip-extensions")

	if onlyExtensions != "" && skipExtensions != "" {
		return fmt.Errorf("only-extensions and skip-extensions cannot be used together")
	}

	if onlyExtensions != "" {
		cfgs = cfgs.Only(strings.Split(onlyExtensions, ","))
	}

	if skipExtensions != "" {
		cfgs = cfgs.Not(strings.Split(skipExtensions, ","))
	}

	if _, err := extension.InstallNodeModulesOfConfigs(cmd.Context(), cfgs, false); err != nil {
		return err
	}

	pluginJson, err := json.MarshalIndent(cfgs, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path.Join(projectRoot, "var", "plugins.json"), pluginJson, os.ModePerm); err != nil {
		return err
	}

	return nil
}

package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/internal/asset"
	"github.com/shopware/shopware-cli/internal/phpexec"
	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
)

func findClosestShopwareProject() (string, error) {
	projectRoot := os.Getenv("PROJECT_ROOT")

	if projectRoot != "" {
		return projectRoot, nil
	}

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

func isShopwareProject(projectRoot string) bool {
	files := []string{
		fmt.Sprintf("%s/composer.json", projectRoot),
		fmt.Sprintf("%s/composer.lock", projectRoot),
	}

	for _, file := range files {
		if _, err := os.Stat(file); err == nil {
			content, err := os.ReadFile(file)
			if err != nil {
				continue
			}
			contentString := string(content)

			if strings.Contains(contentString, "shopware/core") {
				if _, err := os.Stat(fmt.Sprintf("%s/bin/console", projectRoot)); err == nil {
					return true
				}
			}
		}
	}

	return false
}

func findProjectRoot(configPath string) (string, error) {
	// Check if we have a custom config path that's not the default
	defaultConfigName := shop.DefaultConfigFileName()

	// If configPath is provided and is not just the default filename (relative path),
	// try to derive project root from the config path
	if configPath != "" && configPath != defaultConfigName && filepath.IsAbs(configPath) {
		// Get the directory containing the config file
		configDir := filepath.Dir(configPath)

		// Check if this directory is a Shopware project
		if isShopwareProject(configDir) {
			return configDir, nil
		}
	}

	// Fall back to the existing behavior
	return findClosestShopwareProject()
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
	} else {
		logging.FromContext(cmd.Context()).Infof("Excluding extensions based on project config: %s", strings.Join(shopCfg.Build.ExcludeExtensions, ", "))
		cfgs = cfgs.Not(shopCfg.Build.ExcludeExtensions)
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

func filterAndGetSources(cmd *cobra.Command, projectRoot string, shopCfg *shop.Config) ([]asset.Source, error) {
	sources, err := extension.DumpAndLoadAssetSourcesOfProject(phpexec.AllowBinCI(cmd.Context()), projectRoot, shopCfg)
	if err != nil {
		return nil, err
	}

	onlyExtensions, _ := cmd.PersistentFlags().GetString("only-extensions")
	skipExtensions, _ := cmd.PersistentFlags().GetString("skip-extensions")
	onlyCustomStatic, _ := cmd.PersistentFlags().GetBool("only-custom-static-extensions")

	if onlyExtensions != "" && skipExtensions != "" {
		return nil, fmt.Errorf("only-extensions and skip-extensions cannot be used together")
	}

	if onlyCustomStatic {
		logging.FromContext(cmd.Context()).Infof("Only including extensions from custom/static-plugins directory")
		logging.FromContext(cmd.Context()).Debugf("Found %d total extensions before filtering", len(sources))
		for _, s := range sources {
			logging.FromContext(cmd.Context()).Debugf("Extension: %s, Path: %s", s.Name, s.Path)
		}

		sources = slices.DeleteFunc(sources, func(s asset.Source) bool {
			// First try to resolve any symlinks
			resolvedPath, err := filepath.EvalSymlinks(s.Path)
			if err != nil {
				logging.FromContext(cmd.Context()).Errorf("Failed to resolve symlink for %s: %v", s.Path, err)
				return true
			}

			absPath, err := filepath.Abs(resolvedPath)
			if err != nil {
				logging.FromContext(cmd.Context()).Errorf("Failed to get absolute path for %s: %v", resolvedPath, err)
				return true
			}

			logging.FromContext(cmd.Context()).Debugf("Extension %s: Original path: %s, Resolved absolute path: %s", s.Name, s.Path, absPath)

			customStaticDir := filepath.Join("custom", "static-plugins")

			isCustomStatic := strings.Contains(absPath, customStaticDir) || strings.HasSuffix(absPath, customStaticDir)

			if !isCustomStatic {
				logging.FromContext(cmd.Context()).Debugf("Excluding %s as it's not in custom/static-plugins", s.Name)
			}
			return !isCustomStatic
		})

		logging.FromContext(cmd.Context()).Debugf("Found %d custom/static extensions after filtering", len(sources))
		for _, s := range sources {
			logging.FromContext(cmd.Context()).Debugf("Included extension: %s, Path: %s", s.Name, s.Path)
		}

		logging.FromContext(cmd.Context()).Debugf("Included extensions:")
		for _, s := range sources {
			logging.FromContext(cmd.Context()).Debugf("  - %s", s.Name)
		}
	}

	if onlyExtensions == "" && skipExtensions == "" && !onlyCustomStatic {
		logging.FromContext(cmd.Context()).Infof("Excluding extensions based on project config: %s", strings.Join(shopCfg.Build.ExcludeExtensions, ", "))
		sources = slices.DeleteFunc(sources, func(s asset.Source) bool {
			return slices.Contains(shopCfg.Build.ExcludeExtensions, s.Name)
		})
	}

	if onlyExtensions != "" {
		logging.FromContext(cmd.Context()).Infof("Only including extensions: %s", onlyExtensions)
		sources = slices.DeleteFunc(sources, func(s asset.Source) bool {
			return !slices.Contains(strings.Split(onlyExtensions, ","), s.Name)
		})
	} else if skipExtensions != "" {
		logging.FromContext(cmd.Context()).Infof("Excluding extensions: %s", skipExtensions)
		sources = slices.DeleteFunc(sources, func(s asset.Source) bool {
			return slices.Contains(strings.Split(skipExtensions, ","), s.Name)
		})
	}

	return sources, nil
}

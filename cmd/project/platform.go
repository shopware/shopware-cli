package project

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/asset"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

const storefrontBundleName = "Storefront"

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

func filterAndWritePluginJson(cmd *cobra.Command, projectRoot string, shopCfg *shop.Config, cmdExecutor executor.Executor) error {
	sources, err := filterAndGetSources(cmd, projectRoot, shopCfg)
	if err != nil {
		return err
	}

	return extension.WritePluginJsonForSources(cmd.Context(), projectRoot, sources, cmdExecutor)
}

func filterAndGetSources(cmd *cobra.Command, projectRoot string, shopCfg *shop.Config) ([]asset.Source, error) {
	cmdExecutor, err := resolveExecutor(cmd, projectRoot)
	if err != nil {
		return nil, err
	}

	sources, err := extension.LoadProjectAssetSources(cmd.Context(), projectRoot, shopCfg, cmdExecutor)
	if err != nil {
		return nil, err
	}

	onlyExtensions, _ := cmd.PersistentFlags().GetString("only-extensions")
	skipExtensions, _ := cmd.PersistentFlags().GetString("skip-extensions")
	onlyCustomStatic, _ := cmd.PersistentFlags().GetBool("only-custom-static-extensions")

	if onlyExtensions != "" && skipExtensions != "" {
		return nil, fmt.Errorf("only-extensions and skip-extensions cannot be used together")
	}

	logger := logging.FromContext(cmd.Context())

	switch {
	case onlyCustomStatic:
		logger.Infof("Only including extensions from custom/static-plugins directory")
		logger.Debugf("Found %d total extensions before filtering", len(sources))
		for _, s := range sources {
			logger.Debugf("Extension: %s, Path: %s", s.Name, s.Path)
		}

		sources = slices.DeleteFunc(sources, func(s asset.Source) bool {
			// Storefront must stay or the watchers break.
			if s.Name == storefrontBundleName {
				return false
			}

			resolvedPath, err := filepath.EvalSymlinks(s.Path)
			if err != nil {
				logger.Errorf("Failed to resolve symlink for %s: %v", s.Path, err)
				return true
			}

			absPath, err := filepath.Abs(resolvedPath)
			if err != nil {
				logger.Errorf("Failed to get absolute path for %s: %v", resolvedPath, err)
				return true
			}

			logger.Debugf("Extension %s: Original path: %s, Resolved absolute path: %s", s.Name, s.Path, absPath)

			customStaticDir := filepath.Join("custom", "static-plugins")
			isCustomStatic := strings.Contains(absPath, customStaticDir) || strings.HasSuffix(absPath, customStaticDir)
			if !isCustomStatic {
				logger.Debugf("Excluding %s as it's not in custom/static-plugins", s.Name)
			}
			return !isCustomStatic
		})

		logger.Debugf("Found %d custom/static extensions after filtering", len(sources))
		for _, s := range sources {
			logger.Debugf("Included extension: %s, Path: %s", s.Name, s.Path)
		}

	case onlyExtensions != "":
		logger.Infof("Only including extensions: %s", onlyExtensions)
		allowed := strings.Split(onlyExtensions, ",")
		sources = slices.DeleteFunc(sources, func(s asset.Source) bool {
			// Storefront must stay or the watchers break.
			if s.Name == storefrontBundleName {
				return false
			}
			return !slices.Contains(allowed, s.Name)
		})

	case skipExtensions != "":
		logger.Infof("Excluding extensions: %s", skipExtensions)
		sources = extension.ExcludeExtensionsFromSources(sources, strings.Split(skipExtensions, ","))

	default:
		logger.Infof("Excluding extensions based on project config: %s", strings.Join(shopCfg.Build.ExcludeExtensions, ", "))
		sources = extension.ExcludeExtensionsFromSources(sources, shopCfg.Build.ExcludeExtensions)
	}

	return sources, nil
}

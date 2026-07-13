package extension

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path"
	"slices"

	"github.com/shopware/shopware-cli/internal/asset"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

const storefrontBundleName = "Storefront"

func LoadProjectAssetSources(ctx context.Context, projectRoot string, shopCfg *shop.Config, cmdExecutor executor.Executor) ([]asset.Source, error) {
	return DumpAndLoadAssetSourcesOfProject(executor.AllowBinCI(ctx), projectRoot, shopCfg, func(ctx context.Context, args ...string) *exec.Cmd {
		return cmdExecutor.ConsoleCommand(ctx, args...).Cmd
	})
}

func ExcludeExtensionsFromSources(sources []asset.Source, excluded []string) []asset.Source {
	if len(excluded) == 0 {
		return sources
	}
	return slices.DeleteFunc(sources, func(s asset.Source) bool {
		// Storefront must stay or the watchers break.
		if s.Name == storefrontBundleName {
			return false
		}
		return slices.Contains(excluded, s.Name)
	})
}

func WriteProjectPluginJson(ctx context.Context, projectRoot string, shopCfg *shop.Config, cmdExecutor executor.Executor) error {
	sources, err := LoadProjectAssetSources(ctx, projectRoot, shopCfg, cmdExecutor)
	if err != nil {
		return err
	}

	sources = ExcludeExtensionsFromSources(sources, shopCfg.Build.ExcludeExtensions)

	return WritePluginJsonForSources(ctx, projectRoot, sources, cmdExecutor)
}

func WritePluginJsonForSources(ctx context.Context, projectRoot string, sources []asset.Source, cmdExecutor executor.Executor) error {
	assetConfig := AssetBuildConfig{
		ShopwareRoot: projectRoot,
		Executor:     cmdExecutor,
	}

	cfgs := BuildAssetConfigFromExtensions(ctx, sources, assetConfig)

	if _, err := InstallNodeModulesOfConfigs(ctx, cfgs, assetConfig); err != nil {
		return err
	}

	// Normalize paths for the execution environment (e.g. Docker container).
	for _, cfg := range cfgs {
		cfg.BasePath = cmdExecutor.NormalizePath(cfg.BasePath)
		for i, v := range cfg.Views {
			cfg.Views[i] = cmdExecutor.NormalizePath(v)
		}
	}

	pluginJson, err := json.MarshalIndent(cfgs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path.Join(projectRoot, "var", "plugins.json"), pluginJson, os.ModePerm)
}

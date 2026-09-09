// Package projectbuild provides the production build pipeline shared by project
// CI and deployment packaging. It does not resolve CLI flags or choose where
// the build runs.
package projectbuild

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/shopware/shopware-cli/internal/asset"
	"github.com/shopware/shopware-cli/internal/ci"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/mjml"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// Options contains build inputs that are not part of the project configuration.
type Options struct {
	WithDevDependencies bool
	ToolVersion         string
}

// Build runs the production build pipeline in root using a local executor.
// It modifies root in place, including removing source files and creating build
// stubs. Callers must either provide a disposable directory or enforce their
// own safety policy before calling Build. Config must be loaded via shop.ReadConfig.
func Build(ctx context.Context, root string, shopCfg *shop.Config, envCfg *shop.EnvironmentConfig, opts Options) error {
	cmdExecutor := executor.NewLocalWithConfig(root, envCfg, shopCfg)
	return run(ctx, root, shopCfg, cmdExecutor, opts)
}

func run(ctx context.Context, root string, shopCfg *shop.Config, cmdExecutor executor.Executor, opts Options) error {
	buildEnv := buildEnvironment(os.Getenv)
	cmdExecutor = cmdExecutor.WithEnv(buildEnv)

	if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.Pre) > 0 {
		if err := executeCIHooks(ctx, "Running pre hooks", shopCfg.Build.Hooks.Pre, root, buildEnv); err != nil {
			return err
		}
	}

	if !shopCfg.DisableComposerInstall {
		composerFlags := []string{"install", "--no-interaction", "--no-progress", "--optimize-autoloader", "--classmap-authoritative"}
		if !opts.WithDevDependencies {
			composerFlags = append(composerFlags, "--no-dev")
		}
		if shopCfg.DisableComposerScripts {
			composerFlags = append(composerFlags, "--no-scripts")
		}
		token, err := prepareComposerAuth(ctx, root)
		if err != nil {
			return err
		}
		if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.PreComposer) > 0 {
			if err := executeCIHooks(ctx, "Running pre-composer hooks", shopCfg.Build.Hooks.PreComposer, root, buildEnv); err != nil {
				return err
			}
		}

		composerInstallSection := ci.Default.Section(ctx, "Composer Installation")
		composer := cmdExecutor.WithEnv(map[string]string{"COMPOSER_AUTH": token}).ComposerCommand(ctx, composerFlags...)
		composer.Cmd.Stdin = os.Stdin
		composer.Cmd.Stdout = os.Stdout
		composer.Cmd.Stderr = os.Stderr
		if err := composer.Run(); err != nil {
			return err
		}
		composerInstallSection.End(ctx)

		if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.PostComposer) > 0 {
			if err := executeCIHooks(ctx, "Running post-composer hooks", shopCfg.Build.Hooks.PostComposer, root, buildEnv); err != nil {
				return err
			}
		}
	} else {
		logging.FromContext(ctx).Infof("Skipping composer install")
	}

	if err := generateProjectSBOM(ctx, root, opts.ToolVersion); err != nil {
		return fmt.Errorf("failed to generate SBOM: %w", err)
	}
	if _, err := os.Stat(path.Join(root, "var", "cache")); err == nil {
		logging.FromContext(ctx).Infof("Removing var/cache")
		if err := os.RemoveAll(path.Join(root, "var", "cache")); err != nil {
			return err
		}
	}

	sources, err := buildAssets(ctx, root, shopCfg, cmdExecutor, buildEnv)
	if err != nil {
		return err
	}
	if err := optimizeAssets(ctx, root, shopCfg, sources); err != nil {
		return err
	}
	if err := warmup(ctx, root, shopCfg, cmdExecutor); err != nil {
		return err
	}
	if err := finalize(ctx, root, shopCfg, sources); err != nil {
		return err
	}
	if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.Post) > 0 {
		return executeCIHooks(ctx, "Running post hooks", shopCfg.Build.Hooks.Post, root, buildEnv)
	}
	return nil
}

func buildAssets(ctx context.Context, root string, shopCfg *shop.Config, cmdExecutor executor.Executor, buildEnv map[string]string) ([]asset.Source, error) {
	lookingForExtensionsSection := ci.Default.Section(ctx, "Looking for extensions")
	sources := extension.FindAssetSourcesOfProject(ctx, root, shopCfg)
	shopwareConstraint, err := extension.GetShopwareProjectConstraint(root)
	if err != nil {
		return nil, err
	}
	lookingForExtensionsSection.End(ctx)

	assetCfg := extension.AssetBuildConfig{
		EnableAssetCaching:           shopCfg.Build.AssetCaching,
		CleanupNodeModules:           true,
		ShopwareRoot:                 root,
		ShopwareVersion:              shopwareConstraint,
		Browserslist:                 shopCfg.Build.Browserslist,
		SkipExtensionsWithBuildFiles: true,
		DisableStorefrontBuild:       shopCfg.Build.DisableStorefrontBuild,
		ForceExtensionBuild:          convertForceExtensionBuild(shopCfg.Build.ForceExtensionBuild),
		ForceAdminBuild:              shopCfg.Build.ForceAdminBuild,
		KeepNodeModules:              shopCfg.Build.KeepNodeModules,
		Executor:                     cmdExecutor,
	}

	if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.PreAssets) > 0 {
		if err := executeCIHooks(ctx, "Running pre-assets hooks", shopCfg.Build.Hooks.PreAssets, root, buildEnv); err != nil {
			return nil, err
		}
	}
	if err := extension.BuildAssetsForExtensions(ctx, sources, assetCfg); err != nil {
		return nil, err
	}
	if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.PostAssets) > 0 {
		if err := executeCIHooks(ctx, "Running post-assets hooks", shopCfg.Build.Hooks.PostAssets, root, buildEnv); err != nil {
			return nil, err
		}
	}
	return sources, nil
}

func optimizeAssets(ctx context.Context, root string, shopCfg *shop.Config, sources []asset.Source) error {
	optimizeSection := ci.Default.Section(ctx, "Optimizing Administration Assets")
	defer optimizeSection.End(ctx)
	if err := extension.CleanupAdministrationFiles(ctx, path.Join(root, "vendor", "shopware", "administration")); err != nil {
		return err
	}
	if err := createEmptySnippetFolder(path.Join(root, "vendor", "shopware", "administration")); err != nil {
		return err
	}
	if !shopCfg.Build.KeepExtensionSource {
		for _, source := range sources {
			if err := extension.CleanupAdministrationFiles(ctx, source.Path); err != nil {
				return err
			}
		}
	}
	if !shopCfg.Build.KeepSourceMaps {
		if err := extension.CleanupJavaScriptSourceMaps(path.Join(root, "vendor", "shopware", "administration", "Resources", "public")); err != nil {
			return err
		}
		for _, source := range sources {
			if err := extension.CleanupJavaScriptSourceMaps(path.Join(source.Path, "Resources", "public")); err != nil {
				return err
			}
		}
	}
	for _, removePath := range buildCleanupPaths(shopCfg.Build) {
		logging.FromContext(ctx).Infof("Removing %s", removePath)
		if err := os.RemoveAll(path.Join(root, removePath)); err != nil {
			return err
		}
	}
	if err := cleanupTcpdf(root, ctx); err != nil {
		return err
	}
	return nil
}

func warmup(ctx context.Context, root string, shopCfg *shop.Config, cmdExecutor executor.Executor) error {
	warmupSection := ci.Default.Section(ctx, "Warming up container cache")
	defer warmupSection.End(ctx)
	if err := RunCommand(binCICommand(ctx, cmdExecutor, "--version")); err != nil {
		return fmt.Errorf("failed to warmup container cache (php bin/ci --version): %w", err)
	}
	if !shopCfg.Build.DisableAssetCopy {
		logging.FromContext(ctx).Infof("Copying extension assets to final public/bundles folder")
		// Delete asset manifest to force a new build.
		manifestPath := path.Join(root, "public", "asset-manifest.json")
		if _, err := os.Stat(manifestPath); err == nil {
			if err := os.Remove(manifestPath); err != nil {
				return err
			}
		}
		if err := RunCommand(binCICommand(ctx, cmdExecutor, "asset:install")); err != nil {
			return fmt.Errorf("failed to install assets (php bin/ci asset:install): %w", err)
		}
	}
	return nil
}

func finalize(ctx context.Context, root string, shopCfg *shop.Config, sources []asset.Source) error {
	if shopCfg.Build.IsMjmlEnabled() {
		mjmlSection := ci.Default.Section(ctx, "Compiling MJML templates")
		extraIncludePaths := shopCfg.Build.MJML.ResolveIncludePaths(root)
		for _, searchPath := range shopCfg.Build.MJML.GetPaths(root) {
			if _, err := os.Stat(searchPath); !os.IsNotExist(err) {
				logging.FromContext(ctx).Infof("Processing MJML files in: %s", searchPath)
				mjmlOpts := mjml.NewCompileOptions(searchPath, shopCfg.Build.MJML.AllowIncludes, extraIncludePaths)
				if err := mjml.ProcessDirectory(ctx, searchPath, mjmlOpts); err != nil {
					logging.FromContext(ctx).Warnf("MJML compilation had issues in %s: %v", searchPath, err)
				}
			} else {
				logging.FromContext(ctx).Debugf("MJML search path does not exist: %s", searchPath)
			}
		}
		mjmlSection.End(ctx)
	}

	if shopCfg.Build.RemoveExtensionAssets {
		deleteAssetsSection := ci.Default.Section(ctx, "Deleting assets of extensions")
		for _, source := range sources {
			if _, err := os.Stat(path.Join(source.Path, "Resources", "public", "administration", "css")); err == nil {
				if err := os.WriteFile(path.Join(source.Path, "Resources", ".administration-css"), []byte{}, 0o644); err != nil {
					return err
				}
			}
			if _, err := os.Stat(path.Join(source.Path, "Resources", "public", "administration", "js")); err == nil {
				if err := os.WriteFile(path.Join(source.Path, "Resources", ".administration-js"), []byte{}, 0o644); err != nil {
					return err
				}
			}
			if err := os.RemoveAll(path.Join(source.Path, "Resources", "public")); err != nil {
				return err
			}
		}
		if err := os.RemoveAll(path.Join(root, "vendor", "shopware", "administration", "Resources", "public")); err != nil {
			return err
		}
		if err := os.WriteFile(path.Join(root, "vendor", "shopware", "administration", "Resources", ".administration-js"), []byte{}, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(path.Join(root, "vendor", "shopware", "administration", "Resources", ".administration-css"), []byte{}, 0o644); err != nil {
			return err
		}
		deleteAssetsSection.End(ctx)
	}

	if !shopCfg.Build.DisableChecksums {
		checksumSection := ci.Default.Section(ctx, "Generating extension checksums")
		extensions := extension.FindExtensionsFromProject(ctx, root, false)
		for _, ext := range extensions {
			extPath := ext.GetPath()
			if shopCfg.Build.KeepExistingChecksums {
				if _, err := os.Stat(path.Join(extPath, "checksum.json")); err == nil {
					logging.FromContext(ctx).Infof("Keeping existing checksum.json for %s", extPath)
					continue
				}
			}
			if err := extension.GenerateChecksumJSON(ctx, extPath, ext); err != nil {
				logging.FromContext(ctx).Warnf("Failed to generate checksum for %s: %v", extPath, err)
			}
		}
		checksumSection.End(ctx)
	}
	return nil
}

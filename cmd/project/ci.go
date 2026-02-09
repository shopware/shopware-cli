package project

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"dario.cat/mergo"
	"github.com/spf13/cobra"
	"golang.org/x/text/language"

	"github.com/shopware/shopware-cli/internal/ci"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/mjml"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/internal/phpexec"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/logging"
)

// cleanupPaths are paths that are not nesscarry for the production build.
var cleanupPaths = []string{
	"vendor/shopware/storefront/Resources/app/storefront/vendor/bootstrap/dist",
	"vendor/shopware/storefront/Resources/app/storefront/test",
	"vendor/shopware/storefront/Test",
	"vendor/shopware/core/Framework/Test",
	"vendor/shopware/core/Content/Test",
	"vendor/shopware/core/Checkout/Test",
	"vendor/shopware/core/System/Test",
	"vendor/tecnickcom/tcpdf/examples",
}

var projectCI = &cobra.Command{
	Use:   "ci",
	Short: "Build Shopware in the CI",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var err error
		args[0], err = filepath.Abs(args[0])
		if err != nil {
			return err
		}

		if os.Getenv("APP_ENV") == "" {
			if err := os.Setenv("APP_ENV", "prod"); err != nil {
				return err
			}
		}

		// speed up composer install, when no version is set
		if os.Getenv("COMPOSER_ROOT_VERSION") == "" {
			if err := os.Setenv("COMPOSER_ROOT_VERSION", "1.0.0"); err != nil {
				return err
			}
		}

		// Remove annoying cache invalidation errors while asset install
		_ = os.Setenv("SHOPWARE_SKIP_ASSET_INSTALL_CACHE_INVALIDATION", "1")

		shopCfg, err := shop.ReadConfig(projectConfigPath, true)
		if err != nil {
			return err
		}

		cleanupPaths = append(cleanupPaths, shopCfg.Build.CleanupPaths...)

		if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.Pre) > 0 {
			if err := executeCIHooks(cmd.Context(), "Running pre hooks", shopCfg.Build.Hooks.Pre, args[0]); err != nil {
				return err
			}
		}

		if !shopCfg.DisableComposerInstall {
			composerFlags := []string{"install", "--no-interaction", "--no-progress", "--optimize-autoloader", "--classmap-authoritative"}

			if withDev, _ := cmd.Flags().GetBool("with-dev-dependencies"); !withDev {
				composerFlags = append(composerFlags, "--no-dev")
			}

			if shopCfg.DisableComposerScripts {
				composerFlags = append(composerFlags, "--no-scripts")
			}

			token, err := prepareComposerAuth(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.PreComposer) > 0 {
				if err := executeCIHooks(cmd.Context(), "Running pre-composer hooks", shopCfg.Build.Hooks.PreComposer, args[0]); err != nil {
					return err
				}
			}

			composerInstallSection := ci.Default.Section(cmd.Context(), "Composer Installation")

			composer := phpexec.ComposerCommand(cmd.Context(), composerFlags...)
			composer.Dir = args[0]
			composer.Stdin = os.Stdin
			composer.Stdout = os.Stdout
			composer.Stderr = os.Stderr
			composer.Env = append(os.Environ(),
				"COMPOSER_AUTH="+token,
			)

			if err := composer.Run(); err != nil {
				return err
			}

			composerInstallSection.End(cmd.Context())

			if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.PostComposer) > 0 {
				if err := executeCIHooks(cmd.Context(), "Running post-composer hooks", shopCfg.Build.Hooks.PostComposer, args[0]); err != nil {
					return err
				}
			}
		} else {
			logging.FromContext(cmd.Context()).Infof("Skipping composer install")
		}

		lookingForExtensionsSection := ci.Default.Section(cmd.Context(), "Looking for extensions")

		sources := extension.FindAssetSourcesOfProject(cmd.Context(), args[0], shopCfg)

		shopwareConstraint, err := extension.GetShopwareProjectConstraint(args[0])
		if err != nil {
			return err
		}

		lookingForExtensionsSection.End(cmd.Context())

		assetCfg := extension.AssetBuildConfig{
			CleanupNodeModules:           true,
			ShopwareRoot:                 args[0],
			ShopwareVersion:              shopwareConstraint,
			Browserslist:                 shopCfg.Build.Browserslist,
			SkipExtensionsWithBuildFiles: true,
			DisableStorefrontBuild:       shopCfg.Build.DisableStorefrontBuild,
			ForceExtensionBuild:          convertForceExtensionBuild(shopCfg.Build.ForceExtensionBuild),
			ForceAdminBuild:              shopCfg.Build.ForceAdminBuild,
			KeepNodeModules:              shopCfg.Build.KeepNodeModules,
		}

		if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.PreAssets) > 0 {
			if err := executeCIHooks(cmd.Context(), "Running pre-assets hooks", shopCfg.Build.Hooks.PreAssets, args[0]); err != nil {
				return err
			}
		}

		if err := extension.BuildAssetsForExtensions(cmd.Context(), sources, assetCfg); err != nil {
			return err
		}

		if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.PostAssets) > 0 {
			if err := executeCIHooks(cmd.Context(), "Running post-assets hooks", shopCfg.Build.Hooks.PostAssets, args[0]); err != nil {
				return err
			}
		}

		optimizeSection := ci.Default.Section(cmd.Context(), "Optimizing Administration Assets")
		if err := cleanupAdministrationFiles(cmd.Context(), path.Join(args[0], "vendor", "shopware", "administration")); err != nil {
			return err
		}

		if err := createEmptySnippetFolder(path.Join(args[0], "vendor", "shopware", "administration")); err != nil {
			return err
		}

		if !shopCfg.Build.KeepExtensionSource {
			for _, source := range sources {
				if err := cleanupAdministrationFiles(cmd.Context(), source.Path); err != nil {
					return err
				}
			}
		}

		if !shopCfg.Build.KeepSourceMaps {
			if err := cleanupJavaScriptSourceMaps(path.Join(args[0], "vendor", "shopware", "administration", "Resources", "public")); err != nil {
				return err
			}

			for _, source := range sources {
				if err := cleanupJavaScriptSourceMaps(path.Join(source.Path, "Resources", "public")); err != nil {
					return err
				}
			}
		}

		for _, removePath := range cleanupPaths {
			logging.FromContext(cmd.Context()).Infof("Removing %s", removePath)

			if err := os.RemoveAll(path.Join(args[0], removePath)); err != nil {
				return err
			}
		}

		if err := cleanupTcpdf(args[0], cmd.Context()); err != nil {
			return err
		}

		optimizeSection.End(cmd.Context())

		warumupSection := ci.Default.Section(cmd.Context(), "Warming up container cache")

		if err := runTransparentCommand(phpexec.PHPCommand(cmd.Context(), path.Join(args[0], "bin", "ci"), "--version")); err != nil { //nolint: gosec
			return fmt.Errorf("failed to warmup container cache (php bin/ci --version): %w", err)
		}

		if !shopCfg.Build.DisableAssetCopy {
			logging.FromContext(cmd.Context()).Infof("Copying extension assets to final public/bundles folder")

			// Delete asset manifest to force a new build
			manifestPath := path.Join(args[0], "public", "asset-manifest.json")
			if _, err := os.Stat(manifestPath); err == nil {
				if err := os.Remove(manifestPath); err != nil {
					return err
				}
			}

			if err := runTransparentCommand(phpexec.PHPCommand(cmd.Context(), path.Join(args[0], "bin", "ci"), "asset:install")); err != nil { //nolint: gosec
				return fmt.Errorf("failed to install assets (php bin/ci asset:install): %w", err)
			}
		}

		warumupSection.End(cmd.Context())

		if _, err := os.Stat(path.Join(args[0], "var", "cache")); err == nil {
			logging.FromContext(cmd.Context()).Infof("Removing var/cache")
			if err := os.RemoveAll(path.Join(args[0], "var", "cache")); err != nil {
				return err
			}
		}

		if shopCfg.Build.IsMjmlEnabled() {
			mjmlSection := ci.Default.Section(cmd.Context(), "Compiling MJML templates")

			for _, searchPath := range shopCfg.Build.MJML.GetPaths(args[0]) {
				if _, err := os.Stat(searchPath); !os.IsNotExist(err) {
					logging.FromContext(cmd.Context()).Infof("Processing MJML files in: %s", searchPath)
					if err := mjml.ProcessDirectory(cmd.Context(), searchPath); err != nil {
						logging.FromContext(cmd.Context()).Warnf("MJML compilation had issues in %s: %v", searchPath, err)
					}
				} else {
					logging.FromContext(cmd.Context()).Debugf("MJML search path does not exist: %s", searchPath)
				}
			}

			mjmlSection.End(cmd.Context())
		}

		if shopCfg.Build.RemoveExtensionAssets {
			deleteAssetsSection := ci.Default.Section(cmd.Context(), "Deleting assets of extensions")

			for _, source := range sources {
				if _, err := os.Stat(path.Join(source.Path, "Resources", "public", "administration", "css")); err == nil {
					if err := os.WriteFile(path.Join(source.Path, "Resources", ".administration-css"), []byte{}, os.ModePerm); err != nil {
						return err
					}
				}

				if _, err := os.Stat(path.Join(source.Path, "Resources", "public", "administration", "js")); err == nil {
					if err := os.WriteFile(path.Join(source.Path, "Resources", ".administration-js"), []byte{}, os.ModePerm); err != nil {
						return err
					}
				}

				if err := os.RemoveAll(path.Join(source.Path, "Resources", "public")); err != nil {
					return err
				}
			}

			if err := os.RemoveAll(path.Join(args[0], "vendor", "shopware", "administration", "Resources", "public")); err != nil {
				return err
			}

			if err := os.WriteFile(path.Join(args[0], "vendor", "shopware", "administration", "Resources", ".administration-js"), []byte{}, os.ModePerm); err != nil {
				return err
			}

			if err := os.WriteFile(path.Join(args[0], "vendor", "shopware", "administration", "Resources", ".administration-css"), []byte{}, os.ModePerm); err != nil {
				return err
			}

			deleteAssetsSection.End(cmd.Context())
		}

		if shopCfg.Build.Hooks != nil && len(shopCfg.Build.Hooks.Post) > 0 {
			if err := executeCIHooks(cmd.Context(), "Running post hooks", shopCfg.Build.Hooks.Post, args[0]); err != nil {
				return err
			}
		}

		return nil
	},
}

func createEmptySnippetFolder(root string) error {
	dirs := []string{
		"Resources/app/administration/src/app/snippet",
		"Resources/app/administration/src/module/dummy/snippet",
		"Resources/app/administration/src/app/component/dummy/dummy/snippet",
	}

	for _, dir := range dirs {
		fullPath := path.Join(root, dir)

		if err := os.MkdirAll(fullPath, os.ModePerm); err != nil {
			return err
		}

		gitkeepPath := path.Join(fullPath, ".gitkeep")
		if err := os.WriteFile(gitkeepPath, []byte{}, 0644); err != nil {
			return err
		}
	}

	return nil
}

func prepareComposerAuth(ctx context.Context, root string) (string, error) {
	auth, err := packagist.ReadComposerAuth(path.Join(root, "auth.json"))

	if err != nil {
		logging.FromContext(ctx).Warnf("Failed to read composer auth from env: %v", err)
		return "", err
	}

	data, err := json.Marshal(auth)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func init() {
	projectRootCmd.AddCommand(projectCI)
	projectCI.PersistentFlags().Bool("with-dev-dependencies", false, "Install dev dependencies")
}

func commandWithRoot(cmd *exec.Cmd, root string) *exec.Cmd {
	cmd.Dir = root

	return cmd
}

func runTransparentCommand(cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "APP_SECRET=b59a3a283700fde2162c0d4f2bcf2588c3e841ef1976cf042d8500c3f3152ec513f77453797387dc004ff399cce0d3663e4fec770e6f11aa4ccd2846854c3a9f", "LOCK_DSN=flock")

	return cmd.Run()
}

func cleanupTcpdf(folder string, ctx context.Context) error {
	tcpdfPath := path.Join(folder, "vendor", "tecnickcom/tcpdf/fonts")

	if _, err := os.Stat(tcpdfPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	logging.FromContext(ctx).Infof("Remove unnecessary fonts from tcpdf")

	return filepath.WalkDir(tcpdfPath, func(path string, d os.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		if filepath.Base(path) == ".z" {
			return os.Remove(path)
		}

		baseName := filepath.Base(path)

		if strings.Contains(baseName, "courier") || strings.Contains(baseName, "helvetica") {
			return nil
		}

		return os.Remove(path)
	})
}

func cleanupAdministrationFiles(ctx context.Context, folder string) error {
	adminFolder := path.Join(folder, "Resources", "app", "administration")

	if _, err := os.Stat(adminFolder); err == nil {
		logging.FromContext(ctx).Infof("Merging Administration snippet for %s", folder)

		snippetFiles := make(map[string][]string)

		err = filepath.WalkDir(adminFolder, func(path string, d os.DirEntry, err error) error {
			if d.IsDir() {
				return nil
			}

			fileExt := filepath.Ext(path)

			if fileExt != ".json" {
				return nil
			}

			languageName := strings.TrimSuffix(filepath.Base(path), fileExt)

			if _, err := language.Parse(languageName); err != nil {
				logging.FromContext(ctx).Infof("Ignoring invalid locale filename %s", path)
				// we can safely ignore the error from language.Parse as we use language.Parse to check and stop processing this file
				// thus checking for the error is the point of this condition
				return nil //nolint:nilerr
			}

			if language.Make(languageName).IsRoot() {
				return nil
			}

			if _, ok := snippetFiles[languageName]; !ok {
				snippetFiles[languageName] = []string{}
			}

			snippetFiles[languageName] = append(snippetFiles[languageName], path)

			return nil
		})
		if err != nil {
			return err
		}

		for language, files := range snippetFiles {
			if len(files) == 1 {
				data, err := os.ReadFile(files[0])
				if err != nil {
					return err
				}

				if err := os.WriteFile(path.Join(folder, language), data, os.ModePerm); err != nil {
					return err
				}

				continue
			}

			merged := make(map[string]interface{})

			for _, file := range files {
				snippetFile := make(map[string]interface{})

				data, err := os.ReadFile(file)
				if err != nil {
					return err
				}

				if err := json.Unmarshal(data, &snippetFile); err != nil {
					return fmt.Errorf("unable to parse %s: %w", file, err)
				}

				if err := mergo.Merge(&merged, snippetFile, mergo.WithOverride); err != nil {
					return err
				}
			}

			mergedData, err := json.Marshal(merged)
			if err != nil {
				return err
			}

			if err := os.WriteFile(path.Join(folder, language), mergedData, os.ModePerm); err != nil {
				return err
			}
		}

		logging.FromContext(ctx).Infof("Deleting Administration source files for %s", folder)

		if err := os.RemoveAll(adminFolder); err != nil {
			return err
		}

		logging.FromContext(ctx).Infof("Migrating generated snippet file for %s", folder)

		snippetFolder := path.Join(adminFolder, "src", "app", "snippet")
		if err := os.MkdirAll(snippetFolder, os.ModePerm); err != nil {
			return err
		}

		for language := range snippetFiles {
			if err := os.Rename(path.Join(folder, language), path.Join(snippetFolder, language+".json")); err != nil {
				return err
			}
		}

		logging.FromContext(ctx).Infof("Creating empty main.js for %s", folder)
		return os.WriteFile(path.Join(adminFolder, "src", "main.js"), []byte(""), os.ModePerm)
	}

	return nil
}

func cleanupJavaScriptSourceMaps(folder string) error {
	if _, err := os.Stat(folder); err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	return filepath.WalkDir(folder, func(path string, d os.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".js.map") {
			return nil
		}

		if err := os.Remove(path); err != nil {
			return err
		}

		expectedJsFile := path[0 : len(path)-4]

		if _, err := os.Stat(expectedJsFile); err != nil {
			if os.IsNotExist(err) {
				return nil
			}

			return err
		}

		content, readErr := os.ReadFile(expectedJsFile)
		if readErr != nil {
			return fmt.Errorf("could not open file %s: %w", expectedJsFile, readErr)
		}

		expectedSourceMapComment := fmt.Sprintf("//# sourceMappingURL=%s", filepath.Base(path))

		overwrittenContent := strings.ReplaceAll(string(content), expectedSourceMapComment, "")

		return os.WriteFile(expectedJsFile, []byte(overwrittenContent), os.ModePerm)
	})
}

func convertForceExtensionBuild(configExtensions []shop.ConfigBuildExtension) []string {
	extensionConfigs := make([]string, len(configExtensions))
	for i, ext := range configExtensions {
		extensionConfigs[i] = ext.Name
	}
	return extensionConfigs
}

func executeCIHooks(ctx context.Context, sectionName string, hooks []string, root string) error {
	section := ci.Default.Section(ctx, sectionName)

	for _, hook := range hooks {
		logging.FromContext(ctx).Infof("Running hook: %s", hook)
		hookCmd := exec.CommandContext(ctx, "sh", "-c", hook)
		hookCmd.Stdout = os.Stdout
		hookCmd.Stderr = os.Stderr
		hookCmd.Dir = root
		hookCmd.Env = append(os.Environ(), fmt.Sprintf("PROJECT_ROOT=%s", root))

		if err := hookCmd.Run(); err != nil {
			return fmt.Errorf("hook failed (%s): %w", hook, err)
		}
	}

	section.End(ctx)

	return nil
}

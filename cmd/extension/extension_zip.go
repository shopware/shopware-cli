package extension

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	cp "github.com/otiai10/copy"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/archiver"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/logging"
)

var (
	disableGit           = false
	extensionReleaseMode = false
)

var extensionZipCmd = &cobra.Command{
	Use:   "zip [path] [branch]",
	Short: "Zip an extension",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		extPath, err := filepath.Abs(args[0])
		if err != nil {
			return err
		}

		var branch string
		if len(args) == 2 {
			branch = args[1]
		}

		ext, err := extension.GetExtensionByFolder(cmd.Context(), extPath)
		if err != nil {
			return fmt.Errorf("detect extension type: %w", err)
		}

		extCfg := ext.GetExtensionConfig()

		name, err := ext.GetName()
		if err != nil {
			return fmt.Errorf("get name: %w", err)
		}

		// Clear previous zips
		existingFiles, err := filepath.Glob(fmt.Sprintf("%s-*.zip", name))
		if err != nil {
			return err
		}

		for _, file := range existingFiles {
			err = os.Remove(file)
			if err != nil {
				return fmt.Errorf("remove existing file: %w", err)
			}
		}

		// Create temp dir
		tempDir, err := os.MkdirTemp("", "extension")
		if err != nil {
			return fmt.Errorf("create temp directory: %w", err)
		}

		extName, err := ext.GetName()
		if err != nil {
			return fmt.Errorf("get extension name: %w", err)
		}

		extDir := fmt.Sprintf("%s/%s/", tempDir, extName)

		err = os.Mkdir(extDir, os.ModePerm)
		if err != nil {
			return fmt.Errorf("create temp directory: %w", err)
		}

		tempDir += "/"

		defer func(path string) {
			_ = os.RemoveAll(path)
		}(tempDir)

		var tag string

		// Extract files using strategy
		if disableGit {
			err = cp.Copy(extPath, extDir, copyOptions())
			if err != nil {
				return fmt.Errorf("copy files: %w", err)
			}
		} else {
			gitCommit, _ := cmd.Flags().GetString("git-commit")

			tag, err = extension.GitCopyFolder(cmd.Context(), extPath, extDir, gitCommit)
			if err != nil {
				return fmt.Errorf("copy via git: %w", err)
			}

			logging.FromContext(cmd.Context()).Infof("Checking out %s using Git", tag)
		}

		// User input wins
		if len(branch) > 0 {
			tag = branch
		}

		if extCfg.Build.Zip.Composer.Enabled {
			if err := executeHooks(cmd.Context(), ext, extCfg.Build.Zip.Composer.BeforeHooks, extDir); err != nil {
				return fmt.Errorf("before hooks composer: %w", err)
			}

			if err := extension.PrepareFolderForZipping(cmd.Context(), extDir, ext, extCfg); err != nil {
				return fmt.Errorf("prepare package: %w", err)
			}

			if err := executeHooks(cmd.Context(), ext, extCfg.Build.Zip.Composer.AfterHooks, extDir); err != nil {
				return fmt.Errorf("after hooks composer: %w", err)
			}
		}
		var tempExt extension.Extension
		if tempExt, err = extension.GetExtensionByFolder(cmd.Context(), extDir); err != nil {
			return err
		}

		if extCfg.Build.Zip.Assets.Enabled {
			if err := executeHooks(cmd.Context(), ext, extCfg.Build.Zip.Assets.BeforeHooks, extDir); err != nil {
				return fmt.Errorf("before hooks assets: %w", err)
			}

			shopwareConstraint, err := tempExt.GetShopwareVersionConstraint()
			if err != nil {
				return fmt.Errorf("get shopware version constraint: %w", err)
			}

			assetBuildConfig := extension.AssetBuildConfig{
				CleanupNodeModules: true,
				ShopwareRoot:       os.Getenv("SHOPWARE_PROJECT_ROOT"),
				ShopwareVersion:    shopwareConstraint,
			}

			if err := extension.BuildAssetsForExtensions(cmd.Context(), extension.ConvertExtensionsToSources(cmd.Context(), []extension.Extension{tempExt}), assetBuildConfig); err != nil {
				return fmt.Errorf("building assets: %w", err)
			}

			if err := executeHooks(cmd.Context(), ext, extCfg.Build.Zip.Assets.AfterHooks, extDir); err != nil {
				return fmt.Errorf("after hooks assets: %w", err)
			}
		}

		if cmd.Flags().Changed("overwrite-app-backend-secret") {
			extCfg.Validation.Ignore = append(extCfg.Validation.Ignore, validation.ToolConfigIgnore{Identifier: "metadata.setup"})
			if err := extCfg.Dump(extDir); err != nil {
				return fmt.Errorf("dump extension config: %w", err)
			}
		}

		// Cleanup not wanted files
		if err := extension.CleanupExtensionFolder(extDir, extCfg.Build.Zip.Pack.Excludes.Paths); err != nil {
			return fmt.Errorf("cleanup package: %w", err)
		}

		if extensionReleaseMode {
			if err := extension.PrepareExtensionForRelease(cmd.Context(), extPath, extDir, ext); err != nil {
				return fmt.Errorf("prepare for release: %w", err)
			}
		}

		if err := extension.ResizeExtensionIcon(cmd.Context(), tempExt); err != nil {
			return fmt.Errorf("resize extension icon: %w", err)
		}

		if err := extension.BuildModifier(ext, extDir, extension.BuildModifierConfig{
			AppBackendUrl:    getStringOnStringError(cmd.Flags().GetString("overwrite-app-backend-url")),
			AppBackendSecret: getStringOnStringError(cmd.Flags().GetString("overwrite-app-backend-secret")),
			Version:          getStringOnStringError(cmd.Flags().GetString("overwrite-version")),
		}); err != nil {
			return fmt.Errorf("build modifier: %w", err)
		}

		fileName, _ := cmd.Flags().GetString("filename")

		if len(fileName) == 0 {
			fileName = fmt.Sprintf("%s-%s.zip", name, tag)
			if len(tag) == 0 {
				fileName = fmt.Sprintf("%s.zip", name)
			}
		}

		outputDir, _ := cmd.Flags().GetString("output-directory")

		if len(outputDir) > 0 {
			if _, err := os.Stat(outputDir); os.IsNotExist(err) {
				if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
					return fmt.Errorf("create output directory: %w", err)
				}
			}

			fileName = path.Join(outputDir, fileName)
		}

		if err := executeHooks(cmd.Context(), ext, extCfg.Build.Zip.Pack.BeforeHooks, extDir); err != nil {
			return fmt.Errorf("before hooks pack: %w", err)
		}

		// Generate checksums.json file before creating the zip
		if err := extension.GenerateChecksumJSON(cmd.Context(), extDir, ext); err != nil {
			return fmt.Errorf("generate checksum.json: %w", err)
		}

		if err := archiver.CreateZip(tempDir, fileName); err != nil {
			return fmt.Errorf("create zip file: %w", err)
		}

		logging.FromContext(cmd.Context()).Infof("Created file %s", fileName)

		return nil
	},
}

func init() {
	extensionRootCmd.AddCommand(extensionZipCmd)
	extensionZipCmd.Flags().BoolVar(&disableGit, "disable-git", false, "Use the source folder as it is")
	extensionZipCmd.Flags().BoolVar(&extensionReleaseMode, "release", false, "Release mode (remove app secrets)")
	extensionZipCmd.Flags().String("overwrite-app-backend-url", "", "Change all URLs in manifest.xml to this URL")
	extensionZipCmd.Flags().String("overwrite-app-backend-secret", "", "Change the secret to this value")
	extensionZipCmd.Flags().String("overwrite-version", "", "Change the extension version to this value")
	extensionZipCmd.Flags().String("output-directory", "", "Output directory for the zip file")
	extensionZipCmd.Flags().String("git-commit", "", "Commit Hash / Tag to use")
	extensionZipCmd.Flags().String("filename", "", "Name of the zip file, if not set it will be generated from the extension name and tag")
}

func getStringOnStringError(val string, _ error) string {
	return val
}

func executeHooks(ctx context.Context, ext extension.Extension, hooks []string, extDir string) error {
	env := []string{
		fmt.Sprintf("EXTENSION_DIR=%s", extDir),
		fmt.Sprintf("ORIGINAL_EXTENSION_DIR=%s", ext.GetPath()),
	}

	for _, hook := range hooks {
		hookCmd := exec.CommandContext(ctx, "sh", "-c", hook)
		hookCmd.Stdout = os.Stdout
		hookCmd.Stderr = os.Stderr
		hookCmd.Dir = extDir
		hookCmd.Env = append(os.Environ(), env...)
		err := hookCmd.Run()
		if err != nil {
			return err
		}
	}

	return nil
}

func copyOptions() cp.Options {
	return cp.Options{
		OnSymlink: func(string) cp.SymlinkAction {
			return cp.Skip
		},
	}
}

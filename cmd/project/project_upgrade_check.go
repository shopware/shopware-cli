package project

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	adminSdk "github.com/friendsofshopware/go-shopware-admin-api-sdk"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/extension"
	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/logging"
	"github.com/shopware/shopware-cli/shop"
)

var projectUpgradeCheckCmd = &cobra.Command{
	Use:   "upgrade-check",
	Short: "Check that installed extensions are compatible with a future Shopware version",
	RunE: func(cmd *cobra.Command, args []string) error {
		var cfg *shop.Config
		var err error
		var shopwareVersion *version.Version
		var extensions map[string]string

		if cfg, err = shop.ReadConfig(projectConfigPath, true); err != nil {
			return err
		}

		if cfg.IsAdminAPIConfigured() {
			logging.FromContext(cmd.Context()).Debugf("Using Shopware Admin API to lookup for available extensions")
			client, err := shop.NewShopClient(cmd.Context(), cfg)
			if err != nil {
				return err
			}

			remoteExtensions, _, err := client.ExtensionManager.ListAvailableExtensions(adminSdk.NewApiContext(cmd.Context()))

			if err != nil {
				return fmt.Errorf("failed to list available extensions: %w", err)
			}

			extensions = make(map[string]string, 0)

			for _, ext := range remoteExtensions {
				extensions[ext.Name] = ext.Version
			}

			shopwareVersion = client.ShopwareVersion
		} else {
			logging.FromContext(cmd.Context()).Debugf("Using local composer.lock to lookup for available extensions")
			shopwareVersion, extensions, err = getLocalExtensions()

			if err != nil {
				return fmt.Errorf("failed to get local extensions: %w", err)
			}
		}

		versions, err := extension.GetShopwareVersions(cmd.Context())
		if err != nil {
			return err
		}

		var possibleVersions []string

		for _, v := range versions {
			ver, err := version.NewVersion(v)
			if err != nil {
				continue
			}

			if strings.Contains(v, "RC") {
				continue
			}

			if ver.LessThan(shopwareVersion) {
				continue
			}

			possibleVersions = append(possibleVersions, v)
		}

		if len(possibleVersions) == 0 {
			fmt.Println("You are on the latest version of Shopware")
			return nil
		}

		var selectedVersion string

		prompt := huh.NewSelect[string]().
			Height(10).
			Title("Select a Shopware version to check compatibility").
			Options(
				huh.NewOptions(possibleVersions...)...,
			).
			Value(&selectedVersion)

		if err := prompt.Run(); err != nil {
			return err
		}

		if selectedVersion == "" {
			return fmt.Errorf("no version selected")
		}

		extensionNames := make([]account_api.UpdateCheckExtension, 0)
		for extName, extVersion := range extensions {

			extensionNames = append(extensionNames, account_api.UpdateCheckExtension{
				Name:    extName,
				Version: extVersion,
			})
		}

		updates, err := account_api.GetFutureExtensionUpdates(cmd.Context(), shopwareVersion.String(), selectedVersion, extensionNames)
		if err != nil {
			return err
		}

		for _, name := range extensionNames {
			found := false
			for _, update := range updates {
				if update.Name == name.Name {
					found = true
					break
				}
			}

			if !found {
				updates = append(updates, account_api.UpdateCheckExtensionCompatibility{
					Name: name.Name,
					Status: account_api.UpdateCheckExtensionCompatibilityStatus{
						Label: "Not available in Store",
					},
				})
			}
		}

		t := table.New().Border(lipgloss.NormalBorder()).Headers("Extension Name", "Compatible")
		for _, update := range updates {
			t.Row(update.Name, update.Status.Label)
		}

		fmt.Println(t.Render())

		return nil
	},
}

func init() {
	projectRootCmd.AddCommand(projectUpgradeCheckCmd)
}

func getLocalExtensions() (*version.Version, map[string]string, error) {
	project, err := findClosestShopwareProject()
	if err != nil {
		return nil, nil, err
	}

	composerLock, err := packagist.ReadComposerLock(path.Join(project, "composer.lock"))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read composer.lock: %w", err)
	}

	corePackage := composerLock.GetPackage("shopware/core")

	if corePackage == nil {
		return nil, nil, fmt.Errorf("shopware/core package not found in composer.lock")
	}

	currentVersion, err := version.NewVersion(strings.TrimPrefix(corePackage.Version, "v"))
	if err != nil {
		return nil, nil, err
	}

	extensions := extension.FindExtensionsFromProject(logging.DisableLogger(context.TODO()), project, false)

	var extensionNames = make(map[string]string, 0)

	for _, ext := range extensions {
		extName, _ := ext.GetName()
		extVersion, err := ext.GetVersion()
		if err != nil {
			extVersion = version.Must(version.NewVersion("1.0.0"))
		}

		extensionNames[extName] = extVersion.String()
	}

	return currentVersion, extensionNames, nil
}

package project

import (
	"fmt"
	"path"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/shyim/go-version"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/extension"
	account_api "github.com/shopware/shopware-cli/internal/account-api"
	"github.com/shopware/shopware-cli/internal/packagist"
	"github.com/shopware/shopware-cli/logging"
)

var projectUpgradeCheckCmd = &cobra.Command{
	Use:   "upgrade-check",
	Short: "Check that installed extensions are compatible with a future Shopware version",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		composerLock, err := packagist.ReadComposerLock(path.Join(project, "composer.lock"))
		if err != nil {
			return err
		}

		corePackage := composerLock.GetPackage("shopware/core")

		if corePackage == nil {
			return fmt.Errorf("shopware/core package not found in composer.lock")
		}

		currentVersion, err := version.NewVersion(strings.TrimPrefix(corePackage.Version, "v"))
		if err != nil {
			return err
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

			if ver.LessThan(currentVersion) {
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

		extensions := extension.FindExtensionsFromProject(logging.DisableLogger(cmd.Context()), project)

		extensionNames := make([]account_api.UpdateCheckExtension, len(extensions))
		for i, ext := range extensions {
			extName, _ := ext.GetName()
			extVersion, err := ext.GetVersion()
			if err != nil {
				extVersion = version.Must(version.NewVersion("1.0.0"))
			}

			extensionNames[i] = account_api.UpdateCheckExtension{
				Name:    extName,
				Version: extVersion.String(),
			}
		}

		updates, err := account_api.GetFutureExtensionUpdates(cmd.Context(), currentVersion.String(), selectedVersion, extensionNames)
		if err != nil {
			return err
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

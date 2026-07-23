package project

import (
	"context"
	"fmt"
	"path"
	"strings"

	"charm.land/huh/v2"
	"charm.land/huh/v2/spinner"
	"github.com/shyim/go-composer"
	"github.com/shyim/go-composer/repository"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
	"github.com/shopware/shopware-cli/internal/tui"
	"github.com/shopware/shopware-cli/logging"
)

var projectAutofixComposerCmd = &cobra.Command{
	Use:   "composer-plugins",
	Short: "Autofix plugins from custom/plugins to Composer",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		rootComposerJson, err := composer.ReadJson(path.Join(project, "composer.json"))
		if err != nil {
			return err
		}

		var token string

		if !system.IsInteractionEnabled(cmd.Context()) {
			return fmt.Errorf("this command requires interaction to enter the Shopware Packagist Token, but interaction is disabled")
		}

		if err := huh.NewInput().
			Title("Please enter the Shopware Packagist Token").
			Value(&token).
			Run(); err != nil {
			return err
		}

		if token == "" {
			return fmt.Errorf("token cannot be empty")
		}

		ctx, cancel := context.WithCancel(cmd.Context())

		go func() {
			_ = spinner.New().Context(ctx).Title("Fetching packages").Run()
		}()

		const storeURL = "https://packages.shopware.com"
		storeAuth := &composer.Auth{BearerAuth: map[string]string{storeURL: token}}
		storePackages, err := repository.New(storeURL, storeAuth).GetPackages(cmd.Context())

		cancel()

		if err != nil {
			return err
		}

		extensions := extension.FindExtensionsFromProject(logging.DisableLogger(cmd.Context()), project, false)

		composerInstall := []string{}
		deleteDirectories := []string{}

		for _, extension := range extensions {
			if !strings.Contains(extension.GetPath(), "custom/plugins") {
				continue
			}

			extName, err := extension.GetName()
			if err != nil {
				return err
			}

			extVersion, err := extension.GetVersion()
			if err != nil {
				return err
			}

			storeName := fmt.Sprintf("store.shopware.com/%s", strings.ToLower(extName))
			if _, inStore := storePackages[storeName]; !inStore {
				composerName, err := extension.GetComposerName()
				if err != nil {
					continue
				}

				if !rootComposerJson.HasPackage(composerName) {
					composerInstall = append(composerInstall, composerName)
				}

				continue
			}

			composerInstall = append(composerInstall, fmt.Sprintf("store.shopware.com/%s:%s", strings.ToLower(extName), extVersion.String()))
			deleteDirectories = append(deleteDirectories, extension.GetPath())
		}

		if len(composerInstall) > 0 {
			fmt.Println("You can install the existing plugins with the following command:")
			fmt.Println(tui.GreenText.Render("composer require " + strings.Join(composerInstall, " ")))
		}

		if len(deleteDirectories) > 0 {
			fmt.Println("and delete the following directories afterwards:")
			fmt.Println(tui.GreenText.Render("rm -rf " + strings.Join(deleteDirectories, " ")))
		}

		fmt.Println("")
		fmt.Print("Don't forget to run ")
		fmt.Print(tui.GreenText.Render("bin/console plugin:refresh"))
		fmt.Println(" after deleting the directories.")

		if !rootComposerJson.Repositories.HasRepository("https://packages.shopware.com") {
			rootComposerJson.Repositories = append(rootComposerJson.Repositories, composer.Repository{
				Type: "composer",
				URL:  "https://packages.shopware.com",
			})
		}

		auth, err := shop.ReadComposerAuth(path.Join(project, "auth.json"))
		if err != nil {
			return err
		}

		auth.BearerAuth["packages.shopware.com"] = token

		if err := auth.Save(); err != nil {
			return err
		}

		return rootComposerJson.Save()
	},
}

func init() {
	projectAutofixCmd.AddCommand(projectAutofixComposerCmd)
}

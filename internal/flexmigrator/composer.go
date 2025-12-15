package flexmigrator

import (
	"path"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func MigrateComposerJson(project string) error {
	composerJson, err := packagist.ReadComposerJson(path.Join(project, "composer.json"))
	if err != nil {
		return err
	}

	if composerJson.HasPackage("shopware/recovery") {
		delete(composerJson.Require, "shopware/recovery")
	}

	composerJson.Require["symfony/flex"] = "^2"
	composerJson.Require["symfony/runtime"] = "*"

	if composerJson.HasPackage("php") {
		delete(composerJson.Require, "php")
	}

	composerJson.RequireDev = packagist.ComposerPackageLink{}
	composerJson.RequireDev["shopware/dev-tools"] = "*"

	if composerJson.HasConfig("platform") {
		delete(composerJson.Config, "platform")
	}

	composerJson.EnableComposerPlugin("symfony/flex")
	composerJson.EnableComposerPlugin("symfony/runtime")
	composerJson.RemoveComposerPlugin("composer/package-versions-deprecated")

	composerJson.Extra["symfony"] = map[string]any{
		"allow-contrib": true,
		"endpoint": []string{
			"https://raw.githubusercontent.com/shopware/recipes/flex/main/index.json",
			"flex://defaults",
		},
	}

	if !composerJson.Repositories.HasRepository("custom/plugins/*") {
		composerJson.Repositories = append(composerJson.Repositories, packagist.ComposerJsonRepository{
			Type: "path",
			URL:  "custom/plugins/*",
			Options: map[string]any{
				"symlink": true,
			},
		})
	}

	if !composerJson.Repositories.HasRepository("custom/plugins/*/packages/*") {
		composerJson.Repositories = append(composerJson.Repositories, packagist.ComposerJsonRepository{
			Type: "path",
			URL:  "custom/plugins/*/packages/*",
			Options: map[string]any{
				"symlink": true,
			},
		})
	}

	composerJson.Scripts = map[string]any{
		"auto-scripts": map[string]string{
			"assets:install": "symfony-cmd",
		},
		"post-install-cmd": []string{
			"@auto-scripts",
		},
		"post-update-cmd": []string{
			"@auto-scripts",
		},
	}

	if err := composerJson.Save(); err != nil {
		return err
	}

	return nil
}

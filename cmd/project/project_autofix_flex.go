package project

import (
	"fmt"
	"os"
	"path"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/color"
	"github.com/shopware/shopware-cli/internal/flexmigrator"
)

var projectAutofixFlexCmd = &cobra.Command{
	Use:   "flex",
	Short: "Autofix project to Symfony Flex",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		var confirmed bool
		if err := huh.NewConfirm().
			Title("Are you sure you want to autofix this project to Symfony Flex?").
			Description("This will modify your composer.json and .env files. Make sure to commit your changes before running this command.").
			Value(&confirmed).
			Run(); err != nil {
			return err
		}

		if !confirmed {
			return fmt.Errorf("autofix cancelled")
		}

		if _, err := os.Stat(path.Join(project, "symfony.lock")); err == nil {
			return fmt.Errorf("symfony.lock already exists, is that project already migrated to Symfony Flex?")
		}

		if err := flexmigrator.MigrateComposerJson(project); err != nil {
			return err
		}

		if err := flexmigrator.MigrateEnv(project); err != nil {
			return err
		}

		if err := flexmigrator.Cleanup(project); err != nil {
			return err
		}

		fmt.Println("Project migrated to Symfony Flex")
		fmt.Printf("Please run %s to install the new dependencies\n", color.GreenText.Render("composer update"))
		fmt.Printf("and %s to apply the recipes\n", color.GreenText.Render("yes | composer recipes:install --reset --force"))

		return nil
	},
}

func init() {
	projectAutofixCmd.AddCommand(projectAutofixFlexCmd)
}

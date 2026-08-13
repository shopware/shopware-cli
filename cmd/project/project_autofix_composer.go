package project

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/shop/pluginmigrate"
	"github.com/shopware/shopware-cli/internal/system"
	pluginmigratetui "github.com/shopware/shopware-cli/internal/tui/pluginmigrate"
)

var projectAutofixComposerCmd = &cobra.Command{
	Use:   "composer-plugins",
	Short: "Migrate extensions from custom/ to Composer management",
	Long: "Migrates the extensions living in custom/ under Composer management: Shopware Store plugins are required from packages.shopware.com and their local copy removed, everything else is registered as a Composer path repository.\n" +
		"In a terminal this runs as an interactive wizard. With --no-interaction (or without a terminal) the migration runs headless: set SHOPWARE_PACKAGIST_TOKEN to migrate Store plugins (otherwise everything becomes a path repository) and use --dry-run to preview the plan.",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		exec, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		if !system.IsInteractionEnabled(cmd.Context()) {
			dryRun, _ := cmd.Flags().GetBool("dry-run")

			return pluginmigrate.NewPluginMigrator(projectRoot, exec).RunHeadless(cmd.Context(), pluginmigrate.HeadlessOptions{
				Token:  os.Getenv("SHOPWARE_PACKAGIST_TOKEN"),
				DryRun: dryRun,
				Out:    cmd.OutOrStdout(),
			})
		}

		_, err = pluginmigratetui.NewApp(pluginmigratetui.Options{
			ProjectRoot:   projectRoot,
			Executor:      exec,
			BackgroundCmd: updateBackgroundCmd(cmd.Context()),
		}).Run()
		return err
	},
}

func init() {
	projectAutofixCmd.AddCommand(projectAutofixComposerCmd)
	projectAutofixComposerCmd.Flags().Bool("dry-run", false, "non-interactive mode: print the migration plan without modifying the project")
}

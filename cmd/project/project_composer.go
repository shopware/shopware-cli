package project

import (
	"github.com/spf13/cobra"
)

var projectComposerCmd = &cobra.Command{
	Use:   "composer",
	Short: "Runs Composer for the current project",
	Long:  "Proxies all arguments to Composer using the project's configured executor (local, Docker, or Symfony CLI).",
	Example: `  shopware-cli project composer install
  shopware-cli project composer require shopware/dev-tools
  shopware-cli project composer update --with-all-dependencies`,
	DisableFlagParsing: true,
	ValidArgsFunction: func(cmd *cobra.Command, input []string, _ string) ([]string, cobra.ShellCompDirective) {
		projectRoot, err := findClosestShopwareProject(false)
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}

		return composerCommandCompletions(cmd, projectRoot, input, cmdExecutor)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject(false)
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		return runExecutorProcess(cmd, cmdExecutor.ComposerCommand(cmd.Context(), args...))
	},
}

func init() {
	projectRootCmd.AddCommand(projectComposerCmd)
}

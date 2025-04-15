package project

import (
	"slices"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/extension"
	"github.com/shopware/shopware-cli/internal/phpexec"
	"github.com/shopware/shopware-cli/shop"
)

var (
	pluginCommands = []string{"plugin:install", "plugin:uninstall", "plugin:update", "plugin:activate", "plugin:deactivate"}
	appCommands    = []string{"app:install", "app:update", "app:activate", "app:deactivate"}
)

var projectConsoleCmd = &cobra.Command{
	Use:                "console",
	Short:              "Runs the Symfony Console (bin/console) for current project",
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	ValidArgsFunction: func(cmd *cobra.Command, input []string, _ string) ([]string, cobra.ShellCompDirective) {
		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}

		parsedCommands, err := shop.GetConsoleCompletion(cmd.Context(), projectRoot)
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}
		completions := make([]string, 0)

		if len(input) == 0 {
			for _, command := range parsedCommands.Commands {
				if !command.Hidden {
					completions = append(completions, command.Name)
				}
			}
		} else {
			completions = parsedCommands.GetCommandOptions(input[0])

			isAppCommand := slices.Contains(appCommands, input[0])
			isPluginCommand := slices.Contains(pluginCommands, input[0])

			if isAppCommand || isPluginCommand {
				extensions := extension.FindExtensionsFromProject(cmd.Context(), projectRoot)

				for _, extension := range extensions {
					if (extension.GetType() == "plugin" && isPluginCommand) || (extension.GetType() == "app" && isAppCommand) {
						name, err := extension.GetName()
						if err != nil {
							continue
						}

						completions = append(completions, name)
					}
				}
			}

			filtered := make([]string, 0)
			for _, completion := range completions {
				if slices.Contains(input, completion) {
					continue
				}

				filtered = append(filtered, completion)
			}

			completions = filtered
		}

		return completions, cobra.ShellCompDirectiveDefault
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		consoleCmd := phpexec.ConsoleCommand(cmd.Context(), args...)
		consoleCmd.Dir = projectRoot
		consoleCmd.Stdin = cmd.InOrStdin()
		consoleCmd.Stdout = cmd.OutOrStdout()
		consoleCmd.Stderr = cmd.ErrOrStderr()

		return consoleCmd.Run()
	},
}

func init() {
	projectRootCmd.AddCommand(projectConsoleCmd)
}

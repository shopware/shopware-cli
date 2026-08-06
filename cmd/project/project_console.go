package project

import (
	"context"
	"fmt"
	"os/exec"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
)

var (
	pluginCommands = []string{"plugin:install", "plugin:uninstall", "plugin:update", "plugin:activate", "plugin:deactivate"}
	appCommands    = []string{"app:install", "app:update", "app:activate", "app:deactivate"}
)

// stripConsoleFlags extracts shopware-cli's own flags (--env/-e and
// --project-config) from the arguments before the first positional argument
// and applies them. Everything from the first positional argument on — the
// console command name — is passed to bin/console untouched, so Symfony's own
// --env/-e flag keeps working there. A leading "--" ends flag handling early.
func stripConsoleFlags(args []string) []string {
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			return append(rest, args[i+1:]...)
		}

		if !strings.HasPrefix(arg, "-") {
			return append(rest, args[i:]...)
		}

		switch {
		case arg == "--env" || arg == "-e":
			if i+1 < len(args) {
				environmentName = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--env="):
			environmentName = strings.TrimPrefix(arg, "--env=")
		case strings.HasPrefix(arg, "-e="):
			environmentName = strings.TrimPrefix(arg, "-e=")
		case arg == "--project-config":
			if i+1 < len(args) {
				projectConfigPath = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--project-config="):
			projectConfigPath = strings.TrimPrefix(arg, "--project-config=")
		default:
			rest = append(rest, arg)
		}
	}

	return rest
}

var projectConsoleCmd = &cobra.Command{
	Use:   "console [flags] -- <command> [console-args]",
	Short: "Runs the Symfony Console (bin/console) for current project",
	Long: `Runs the Symfony Console (bin/console) for the current project.

Flags for shopware-cli (--env/-e to pick an environment, --project-config) must
be placed before the console command name; everything after it is passed to
bin/console unchanged, including Symfony's own --env flag.`,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: true,
	ValidArgsFunction: func(cmd *cobra.Command, input []string, _ string) ([]string, cobra.ShellCompDirective) {
		input = stripConsoleFlags(input)

		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return nil, cobra.ShellCompDirectiveDefault
		}

		parsedCommands, err := shop.GetConsoleCompletion(cmd.Context(), projectRoot, func(ctx context.Context, args ...string) *exec.Cmd {
			return cmdExecutor.ConsoleCommand(ctx, args...).Cmd
		})
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
				extensions := extension.FindExtensionsFromProject(cmd.Context(), projectRoot, false)

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
		args = stripConsoleFlags(args)
		if len(args) == 0 {
			return fmt.Errorf("no console command given")
		}

		projectRoot, err := findClosestShopwareProject()
		if err != nil {
			return err
		}

		cmdExecutor, err := resolveExecutor(cmd, projectRoot)
		if err != nil {
			return err
		}

		p := cmdExecutor.ConsoleCommand(cmd.Context(), args...)
		p.Cmd.Stdin = cmd.InOrStdin()
		p.Cmd.Stdout = cmd.OutOrStdout()
		p.Cmd.Stderr = cmd.ErrOrStderr()

		return p.Run()
	},
}

func init() {
	projectRootCmd.AddCommand(projectConsoleCmd)
}

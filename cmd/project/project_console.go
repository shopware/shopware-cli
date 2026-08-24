package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/extension"
	"github.com/shopware/shopware-cli/internal/shop"
)

var (
	pluginCommands          = []string{"plugin:install", "plugin:uninstall", "plugin:update", "plugin:activate", "plugin:deactivate"}
	appCommands             = []string{"app:install", "app:update", "app:activate", "app:deactivate"}
	reservedConsoleCommands = []string{"list", "help", "about", "completion", "composer"}
)

var projectConsoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Runs Symfony Console commands or Composer scripts for the current project",
	Long: "Runs bin/console for the current project. Custom scripts from the project composer.json " +
		"are also available by name (for example via the swx alias).",
	Args:               cobra.MinimumNArgs(1),
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

		parsedCommands, err := shop.GetConsoleCompletion(cmd.Context(), projectRoot, func(ctx context.Context, args ...string) *exec.Cmd {
			return cmdExecutor.ConsoleCommand(ctx, args...).Cmd
		})
		scripts, _ := shop.GetComposerScripts(projectRoot)
		if err != nil && len(scripts) == 0 {
			return nil, cobra.ShellCompDirectiveDefault
		}

		completions := make([]string, 0)

		if len(input) == 0 {
			completions = append(completions, completionWithDescription("composer", "Run Composer"))
			completions = append(completions, commandListCompletions(parsedCommands)...)

			for _, script := range scripts {
				if parsedCommands != nil && parsedCommands.HasCommand(script.Name) {
					continue
				}

				completions = append(completions, completionWithDescription(script.Name, script.Description))
				for _, alias := range script.Aliases {
					if parsedCommands != nil && parsedCommands.HasCommand(alias) {
						continue
					}

					completions = append(completions, completionWithDescription(alias, script.Description))
				}
			}
		} else if isComposerProxy(input) {
			return composerCommandCompletions(cmd, projectRoot, input[1:], cmdExecutor)
		} else {
			if parsedCommands != nil {
				completions = parsedCommands.GetCommandOptions(input[0])
			}

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

			completions = filterUsedCompletions(completions, input)
		}

		return completions, cobra.ShellCompDirectiveDefault
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

		ctx := consoleCommandContext(cmd.Context())

		if isComposerProxy(args) {
			return runExecutorProcess(cmd, cmdExecutor.ComposerCommand(ctx, args[1:]...))
		}

		scripts := composerScripts(projectRoot)
		if shouldRunComposerScript(args[0], loadConsoleCommands(cmd.Context(), projectRoot, cmdExecutor), scripts) {
			script, ok := shop.FindComposerScript(scripts, args[0])
			if !ok {
				return fmt.Errorf("composer script %q not found", args[0])
			}

			return runExecutorProcess(cmd, cmdExecutor.ComposerCommand(ctx, composerScriptArgs(script.Name, args[1:])...))
		}

		p := cmdExecutor.ConsoleCommand(ctx, args...)
		if err := runExecutorProcess(cmd, p); err != nil {
			return err
		}

		if len(args) == 1 && args[0] == "list" {
			_, err := fmt.Fprint(cmd.OutOrStdout(), formatComposerScriptsList(scripts))
			return err
		}

		return nil
	},
}

// consoleCommandContext requests a compose TTY when the user is on an
// interactive terminal so Symfony console keeps colors and prompts.
func consoleCommandContext(ctx context.Context) context.Context {
	if isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd()) {
		return executor.WithTTY(ctx)
	}

	return ctx
}

func init() {
	projectRootCmd.AddCommand(projectConsoleCmd)
}

func isComposerProxy(args []string) bool {
	return len(args) > 0 && args[0] == "composer"
}

func shouldRunComposerScript(name string, console *shop.ConsoleResponse, scripts []shop.ComposerScript) bool {
	if slices.Contains(reservedConsoleCommands, name) {
		return false
	}

	if console != nil && console.HasCommand(name) {
		return false
	}

	_, ok := shop.FindComposerScript(scripts, name)
	return ok
}

func composerScriptArgs(name string, extra []string) []string {
	args := []string{"run-script", "--timeout=0", "--", name}
	return append(args, extra...)
}

func completionWithDescription(name, description string) string {
	if description == "" {
		return name
	}

	return name + "\t" + description
}

func formatComposerScriptsList(scripts []shop.ComposerScript) string {
	if len(scripts) == 0 {
		return ""
	}

	width := 24
	for _, script := range scripts {
		if n := len(script.Name) + 2; n > width {
			width = n
		}
	}

	var b strings.Builder
	b.WriteString("\n composer\n")
	for _, script := range scripts {
		if script.Description != "" {
			fmt.Fprintf(&b, "  %-*s %s\n", width, script.Name, script.Description)
			continue
		}

		fmt.Fprintf(&b, "  %s\n", script.Name)
	}

	return b.String()
}

func loadConsoleCommands(ctx context.Context, projectRoot string, cmdExecutor executor.Executor) *shop.ConsoleResponse {
	resp, err := shop.GetConsoleCompletion(ctx, projectRoot, func(ctx context.Context, args ...string) *exec.Cmd {
		return cmdExecutor.ConsoleCommand(ctx, args...).Cmd
	})
	if err != nil {
		return nil
	}

	return resp
}

func composerScripts(projectRoot string) []shop.ComposerScript {
	scripts, err := shop.GetComposerScripts(projectRoot)
	if err != nil {
		return nil
	}

	return scripts
}

func composerCommandCompletions(cmd *cobra.Command, projectRoot string, input []string, cmdExecutor executor.Executor) ([]string, cobra.ShellCompDirective) {
	parsedCommands, err := shop.GetComposerCompletion(cmd.Context(), projectRoot, func(ctx context.Context, args ...string) *exec.Cmd {
		return cmdExecutor.ComposerCommand(ctx, args...).Cmd
	})
	if err != nil {
		return nil, cobra.ShellCompDirectiveDefault
	}

	if len(input) == 0 {
		return commandListCompletions(parsedCommands), cobra.ShellCompDirectiveDefault
	}

	return filterUsedCompletions(parsedCommands.GetCommandOptions(input[0]), input), cobra.ShellCompDirectiveDefault
}

func commandListCompletions(parsedCommands *shop.ConsoleResponse) []string {
	if parsedCommands == nil {
		return nil
	}

	completions := make([]string, 0, len(parsedCommands.Commands))
	for _, command := range parsedCommands.Commands {
		if !command.Hidden {
			completions = append(completions, completionWithDescription(command.Name, command.Description))
		}
	}

	return completions
}

func filterUsedCompletions(completions, input []string) []string {
	filtered := make([]string, 0, len(completions))
	for _, completion := range completions {
		if slices.Contains(input, completion) {
			continue
		}

		filtered = append(filtered, completion)
	}

	return filtered
}

func runExecutorProcess(cmd *cobra.Command, p *executor.Process) error {
	p.Cmd.Stdin = cmd.InOrStdin()
	p.Cmd.Stdout = cmd.OutOrStdout()
	p.Cmd.Stderr = cmd.ErrOrStderr()

	return p.Run()
}

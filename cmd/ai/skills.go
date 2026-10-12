package ai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/installer"
)

func addSkillFlags(cmd *cobra.Command) {
	cmd.Flags().String("client", "", "Agent name passed directly to skills; omit to let skills detect clients")
	cmd.Flags().Bool("global", false, "Use the user directory instead of the current project")
	cmd.Flags().Bool("dry-run", false, "Preview the delegated command without changing files")
	addFormatFlag(cmd)
}

func runSkillCommand(action string) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		format, err := resolveFormat(cmd)
		if err != nil {
			return err
		}
		client, _ := cmd.Flags().GetString("client")
		global, _ := cmd.Flags().GetBool("global")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		result, err := installer.Run(cmd.Context(), action, args[0], installer.Options{
			Client: client, Global: global, DryRun: dryRun,
		}, cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		if format == formatJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		if dryRun {
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Would run in %s: %s\n", result.Directory, strings.Join(result.Command, " "))
		} else {
			verb := "Installed"
			if action == "remove" {
				verb = "Removed"
			}
			target := ""
			if result.Client != "" {
				target = " for " + result.Client
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s %s%s (%s)\n", verb, result.Name, target, result.Scope)
		}
		return err
	}
}

package ai

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/install"
)

var aiRemoveCmd = &cobra.Command{
	Use:   "remove name",
	Short: "Remove a Shopware AI integration from an AI agent",
	Long: `Remove a Shopware AI integration from an AI agent via skills.sh.

Use --global to remove a user-level install, otherwise the command operates on the
current Shopware project. Requires Node.js/npx on PATH.`,
	Example: `  shopware-cli ai remove shopware-cli --agent claude-code
  shopware-cli ai remove shopware-cli --agent claude-code --global`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		agent, _ := cmd.Flags().GetString("agent")
		global, _ := cmd.Flags().GetBool("global")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		result, err := install.Remove(cmd.Context(), install.RemoveOptions{
			Name:   args[0],
			Agent:  agent,
			Global: global,
			DryRun: dryRun,
		}, cmd.ErrOrStderr())
		if err != nil {
			return err
		}

		return writeRemoveResult(cmd.OutOrStdout(), result)
	},
}

func writeRemoveResult(w io.Writer, r install.RemoveResult) error {
	// An MCP server lives in the project's .mcp.json, not an agent, so it carries no
	// agent to name.
	agent := ""
	if r.Agent != "" {
		agent = " for " + r.Agent
	}

	if !r.Removed {
		_, err := fmt.Fprintf(w, "%s is not installed%s (%s); nothing to remove\n", r.Name, agent, r.Scope)

		return err
	}

	verb := "Removed"
	if r.DryRun {
		verb = "[dry-run] would remove"
	}
	_, err := fmt.Fprintf(w, "%s %s%s (%s):\n  %s\n", verb, r.Name, agent, r.Scope, strings.Join(r.Command, " "))

	return err
}

func init() {
	aiRootCmd.AddCommand(aiRemoveCmd)
	aiRemoveCmd.Flags().String("agent", "", "Target AI agent (e.g. claude-code)")
	aiRemoveCmd.Flags().Bool("global", false, "Remove the user-level install instead of the current project")
	aiRemoveCmd.Flags().Bool("dry-run", false, "Show what would be removed without changing anything")
}

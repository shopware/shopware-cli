package ai

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/install"
	"github.com/shopware/shopware-cli/internal/tracking"
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
		trackAIRemove(cmd.Context(), result, err)
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

// trackAIRemove emits the ai.remove usage event for a resolved integration. An
// input or guard error returns a zero RemoveResult (nothing was resolved); those
// runs are already covered by shopware_cli.command, so nothing is sent here.
func trackAIRemove(ctx context.Context, r install.RemoveResult, runErr error) {
	if r.Name == "" {
		return
	}

	result := tracking.ResultSuccess
	if runErr != nil {
		result = tracking.ResultFailure
	}

	tags := map[string]string{
		tracking.TagIntegration: r.Name,
		tracking.TagType:        r.Type.String(),
		tracking.TagScope:       r.Scope,
		tracking.TagDryRun:      strconv.FormatBool(r.DryRun),
		tracking.TagRemoved:     strconv.FormatBool(r.Removed),
		tracking.TagResult:      result,
	}
	if r.Type == directory.TypeSkill {
		tags[tracking.TagAgent] = r.Agent
	}

	go tracking.Track(ctx, tracking.EventAIRemove, tags)
}

func init() {
	aiRootCmd.AddCommand(aiRemoveCmd)
	aiRemoveCmd.Flags().String("agent", "", "Target AI agent (e.g. claude-code)")
	aiRemoveCmd.Flags().Bool("global", false, "Remove the user-level install instead of the current project")
	aiRemoveCmd.Flags().Bool("dry-run", false, "Show what would be removed without changing anything")
}

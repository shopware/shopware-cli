package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/install"
)

var aiAddCmd = &cobra.Command{
	Use:   "add name[@tag]",
	Short: "Install a Shopware AI integration into an AI agent",
	Long: `Install a Shopware AI integration (a skill) into an AI agent via skills.sh.

By default the skill is installed into the current Shopware project (a shop); use
--global to install at the user level from anywhere (the right choice in an
extension repository, which is not a shop). Pin a release with @<version>,
otherwise the repository's default branch is used (bundled skills follow the CLI
version).

Requires Node.js/npx on PATH; git-delivered integrations also need git and network
access. The agent name is whatever skills.sh supports (e.g. claude-code, codex) —
the CLI keeps no list of its own.

The install writes the agent's skill files plus skills-lock.json, and records the
install under .shopware-cli/ai/. Commit those to share the integration with your
team, or gitignore them to keep it local.`,
	Example: `  # install the Shopware CLI skill into Claude Code for this project
  shopware-cli ai add shopware-cli --agent claude-code

  # pin a specific release
  shopware-cli ai add deployment-helper@0.1.9 --agent claude-code

  # install at the user level, from anywhere
  shopware-cli ai add shopware-cli --agent claude-code --global

  # preview the exact skills.sh command without changing anything
  shopware-cli ai add shopware-cli --agent claude-code --dry-run`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveFormat(cmd)
		if err != nil {
			return err
		}

		agent, _ := cmd.Flags().GetString("agent")
		global, _ := cmd.Flags().GetBool("global")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		result, err := install.Add(cmd.Context(), install.Options{
			Name:   args[0],
			Agent:  agent,
			Global: global,
			DryRun: dryRun,
		}, cmd.ErrOrStderr())
		if err != nil {
			return err
		}

		return writeAddResult(cmd.OutOrStdout(), format, result)
	},
}

func writeAddResult(w io.Writer, format string, r install.Result) error {
	if format == formatJSON {
		out, err := json.Marshal(r)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(out))

		return err
	}

	dest := ""
	if r.Dir != "" {
		dest = " into " + r.Dir
	}

	if r.DryRun {
		_, err := fmt.Fprintf(w, "[dry-run] would install %s for %s (%s)%s:\n  %s\n", r.Name, r.Agent, r.Scope, dest, strings.Join(r.Command, " "))

		return err
	}

	rev := ""
	if r.ResolvedRevision != "" {
		rev = " @" + r.ResolvedRevision
	}

	switch r.Action {
	case install.ActionUnchanged:
		_, err := fmt.Fprintf(w, "Already installed %s for %s (%s)%s%s\n", r.Name, r.Agent, r.Scope, dest, rev)

		return err
	case install.ActionUpdated:
		_, err := fmt.Fprintf(w, "Updated %s for %s (%s)%s: %s → %s\n", r.Name, r.Agent, r.Scope, dest, r.PreviousRevision, r.ResolvedRevision)

		return err
	default:
		_, err := fmt.Fprintf(w, "Installed %s for %s (%s)%s%s\n", r.Name, r.Agent, r.Scope, dest, rev)

		return err
	}
}

func init() {
	aiRootCmd.AddCommand(aiAddCmd)
	aiAddCmd.Flags().String("agent", "", "Target AI agent (e.g. claude-code)")
	aiAddCmd.Flags().Bool("global", false, "Install at user level instead of the current project")
	aiAddCmd.Flags().Bool("dry-run", false, "Show what would be installed without changing anything")
	addFormatFlag(aiAddCmd)
}

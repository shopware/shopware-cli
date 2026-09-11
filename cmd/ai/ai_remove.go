package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
)

// removeResult is the machine-readable shape of `ai remove` (--format json).
type removeResult struct {
	Name    string      `json:"name"`
	Agent   string      `json:"agent"`
	Scope   state.Scope `json:"scope"`
	Removed bool        `json:"removed"`
	DryRun  bool        `json:"dryRun"`
	Command []string    `json:"command"`
}

var aiRemoveCmd = &cobra.Command{
	Use:          "remove <name>",
	Short:        "Remove a Shopware AI integration the CLI installed",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveFormat(cmd)
		if err != nil {
			return err
		}

		name := args[0]
		agent, _ := cmd.Flags().GetString("agent")
		global, _ := cmd.Flags().GetBool("global")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if agent == "" {
			return errors.New("specify the target agent with --agent (e.g. --agent claude-code)")
		}

		// The directory gives the canonical name, but a recorded install is
		// authority enough to remove — even if the integration has since left
		// the directory.
		entry, known := directory.Load().Get(name)
		removeName := name
		if known {
			removeName = entry.Name
		}

		// State (and the agent config) live in the user config dir for a
		// --global install, or in the current directory for a project install.
		scope := state.ScopeGlobal
		readState := state.Read
		saveState := state.Save
		if !global {
			root, err := os.Getwd()
			if err != nil {
				return err
			}
			scope = state.ScopeProject
			readState = func() (state.File, error) { return state.ReadProject(root) }
			saveState = func(f state.File) error { return state.SaveProject(root, f) }
		}

		current, err := readState()
		if err != nil {
			return err
		}

		// Remove only what the CLI recorded; a hand-written config is left alone.
		next, recorded := state.Remove(current, removeName, agent, scope)

		// Nothing recorded and the name is unknown to the directory: a typo
		// rather than a stale install.
		if !recorded && !known {
			return fmt.Errorf("unknown integration %q (see `shopware-cli ai list`)", name)
		}

		result := removeResult{
			Name:    removeName,
			Agent:   agent,
			Scope:   scope,
			Removed: recorded,
			DryRun:  dryRun,
			Command: skillsRemoveArgs(removeName, agent, global),
		}

		if dryRun || !recorded {
			return writeRemoveResult(cmd.OutOrStdout(), format, result)
		}

		if err := runSkills(cmd.Context(), result.Command, cmd.ErrOrStderr()); err != nil {
			return err
		}
		if err := saveState(next); err != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s was removed but the state file could not be updated (%v); re-run `ai remove`\n", result.Name, err)
			return err
		}

		return writeRemoveResult(cmd.OutOrStdout(), format, result)
	},
}

func writeRemoveResult(w io.Writer, format string, r removeResult) error {
	if format == formatJSON {
		out, err := json.Marshal(r)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(out))

		return err
	}

	if !r.Removed {
		_, err := fmt.Fprintf(w, "%s is not recorded for %s (%s) by shopware-cli; nothing to remove\n", r.Name, r.Agent, r.Scope)

		return err
	}

	verb := "Removed"
	if r.DryRun {
		verb = "[dry-run] would remove"
	}
	_, err := fmt.Fprintf(w, "%s %s for %s (%s):\n  %s\n", verb, r.Name, r.Agent, r.Scope, strings.Join(r.Command, " "))

	return err
}

func init() {
	aiRootCmd.AddCommand(aiRemoveCmd)
	aiRemoveCmd.Flags().String("agent", "", "Target AI agent (e.g. claude-code)")
	aiRemoveCmd.Flags().Bool("global", false, "Remove the user-level install instead of the current project")
	aiRemoveCmd.Flags().Bool("dry-run", false, "Show what would be removed without changing anything")
	addFormatFlag(aiRemoveCmd)
}

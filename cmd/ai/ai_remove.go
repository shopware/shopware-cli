package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
	"github.com/shopware/shopware-cli/internal/shop"
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
	Use:   "remove name",
	Short: "Remove a Shopware AI integration the CLI installed",
	Long: `Remove a Shopware AI integration that this CLI installed, via skills.sh.

Only installs recorded by 'ai add' are removed; a hand-written agent config is
left alone. Use --global to remove a user-level install, otherwise the command
operates on the current Shopware project. Requires Node.js/npx on PATH.`,
	Example: `  shopware-cli ai remove shopware-cli --agent claude-code
  shopware-cli ai remove shopware-cli --agent claude-code --global`,
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

		result, err := performRemove(cmd.Context(), removeOptions{
			name:   args[0],
			agent:  agent,
			global: global,
			dryRun: dryRun,
		}, cmd.ErrOrStderr())
		if err != nil {
			return err
		}

		return writeRemoveResult(cmd.OutOrStdout(), format, result)
	},
}

// removeOptions are the inputs to performRemove, parsed from the command flags.
type removeOptions struct {
	name   string
	agent  string
	global bool
	dryRun bool
}

// performRemove uninstalls via skills.sh and drops the record, but only for what
// the CLI recorded; a hand-written config is left alone.
func performRemove(ctx context.Context, o removeOptions, progress io.Writer) (removeResult, error) {
	if err := validateAgent(o.agent); err != nil {
		return removeResult{}, err
	}

	// A recorded install is enough to remove, even if it left the directory.
	entry, known := directory.Load().Get(o.name)
	removeName := o.name
	if known {
		removeName = entry.Name
	}

	// State (and the agent config) live in the user config dir for a --global
	// install, or at the Shopware project root for a project install.
	scope := state.ScopeGlobal
	projectRoot := ""
	readState := state.Read
	saveState := state.Save
	clearState := state.Clear
	if !o.global {
		root, err := shop.FindClosestShopwareProject(false)
		if err != nil {
			return removeResult{}, fmt.Errorf("a project removal must run inside a Shopware project (or use --global): %w", err)
		}
		scope = state.ScopeProject
		projectRoot = root
		readState = func() (state.File, error) { return state.ReadProject(root) }
		saveState = func(f state.File) error { return state.SaveProject(root, f) }
		clearState = func() error { return state.ClearProject(root) }
	}

	current, err := readState()
	if err != nil {
		return removeResult{}, err
	}

	next, recorded := state.Remove(current, removeName, o.agent, scope)

	// Unknown name with nothing recorded is a typo, not a stale install.
	if !recorded && !known {
		return removeResult{}, fmt.Errorf("unknown integration %q (see \"shopware-cli ai list\")", o.name)
	}

	result := removeResult{
		Name:    removeName,
		Agent:   o.agent,
		Scope:   scope,
		Removed: recorded,
		DryRun:  o.dryRun,
		Command: skillsRemoveArgs(removeName, o.agent, o.global),
	}

	if o.dryRun || !recorded {
		return result, nil
	}

	if err := runSkills(ctx, result.Command, projectRoot, progress); err != nil {
		return removeResult{}, err
	}

	// Remove the state file once empty instead of leaving an empty one.
	save := saveState
	if len(next.Installed) == 0 {
		save = func(state.File) error { return clearState() }
	}
	if err := save(next); err != nil {
		return removeResult{}, fmt.Errorf("%s was removed but its record cannot be updated, re-run \"ai remove\": %w", result.Name, err)
	}

	return result, nil
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

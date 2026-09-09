package ai

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
	"github.com/shopware/shopware-cli/internal/system"
)

// The flag is named --agent to match skills.sh terminology; #1334 refers to it
// as the client. The CLI hardcodes no agent names: which agents exist is
// skills.sh's business, so a new one it supports works without a CLI change.

// addResult is the machine-readable shape of `ai add` (--format json).
type addResult struct {
	Name             string      `json:"name"`
	Client           string      `json:"client"`
	Scope            state.Scope `json:"scope"`
	RequestedTag     string      `json:"requestedTag"`
	ResolvedRevision string      `json:"resolvedRevision"`
	DryRun           bool        `json:"dryRun"`
	Command          []string    `json:"command"`
}

var aiAddCmd = &cobra.Command{
	Use:          "add <name>[@<tag>]",
	Short:        "Install a Shopware AI integration into an AI client",
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := resolveFormat(cmd)
		if err != nil {
			return err
		}

		name, tag := splitNameTag(args[0])
		agent, _ := cmd.Flags().GetString("agent")
		global, _ := cmd.Flags().GetBool("global")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		assumeYes, _ := cmd.Flags().GetBool("yes")

		entry, ok := directory.Load().Get(name)
		if !ok {
			return fmt.Errorf("unknown integration %q (see `shopware-cli ai list`)", name)
		}

		// Scope of this first slice: bundled skills, global scope. The guards
		// below are removed as later slices add project scope and the git
		// delivery path. The agent value is passed straight to skills.sh.
		if entry.Type != directory.TypeSkill || entry.Delivery.Kind != directory.DeliveryBundled {
			return fmt.Errorf("installing %q is not supported yet (only bundled skills for now)", name)
		}
		if !global {
			return errors.New("project-scope install is not available yet; pass --global")
		}
		if agent == "" {
			return errors.New("specify the target client with --agent (e.g. --agent claude-code)")
		}

		// A bundled skill's version follows the CLI, so pin the source to the
		// running CLI version unless the user pinned an explicit tag.
		ref := tag
		if ref == "" {
			ref = cmd.Root().Version
		}
		source := "shopware/shopware-cli"
		if ref != "" {
			source += "@" + ref
		}

		argv := skillsAddArgs(source, entry.Name, agent, global, assumeYes || !system.IsInteractionEnabled(cmd.Context()))

		result := addResult{
			Name:             entry.Name,
			Client:           agent,
			Scope:            state.ScopeGlobal,
			RequestedTag:     tag,
			ResolvedRevision: ref,
			DryRun:           dryRun,
			Command:          argv,
		}

		if dryRun {
			return writeAddResult(cmd.OutOrStdout(), format, result)
		}

		current, err := state.Read()
		if err != nil {
			return err
		}

		// Idempotent: the same integration, client, scope and revision is a no-op.
		if !isInstalled(current, result) {
			if err := runSkills(cmd.Context(), argv); err != nil {
				return err
			}

			next := state.Upsert(current, state.InstalledEntry{
				Name:             result.Name,
				Client:           result.Client,
				Scope:            result.Scope,
				RequestedTag:     result.RequestedTag,
				ResolvedRevision: result.ResolvedRevision,
			})
			if err := state.Save(next); err != nil {
				return err
			}
		}

		return writeAddResult(cmd.OutOrStdout(), format, result)
	},
}

// splitNameTag splits "name@tag" into its parts; a missing tag yields "".
func splitNameTag(s string) (name, tag string) {
	if i := strings.IndexByte(s, '@'); i >= 0 {
		return s[:i], s[i+1:]
	}

	return s, ""
}

// isInstalled reports whether the state already records this exact install.
func isInstalled(f state.File, r addResult) bool {
	for _, e := range f.Installed {
		if e.Name == r.Name && e.Client == r.Client && e.Scope == r.Scope && e.ResolvedRevision == r.ResolvedRevision {
			return true
		}
	}

	return false
}

func writeAddResult(w io.Writer, format string, r addResult) error {
	if format == formatJSON {
		out, err := json.Marshal(r)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(out))

		return err
	}

	if r.DryRun {
		_, err := fmt.Fprintf(w, "[dry-run] would install %s for %s (%s):\n  %s\n", r.Name, r.Client, r.Scope, strings.Join(r.Command, " "))

		return err
	}

	rev := ""
	if r.ResolvedRevision != "" {
		rev = " @" + r.ResolvedRevision
	}
	_, err := fmt.Fprintf(w, "Installed %s for %s (%s)%s\n", r.Name, r.Client, r.Scope, rev)

	return err
}

func init() {
	aiRootCmd.AddCommand(aiAddCmd)
	aiAddCmd.Flags().String("agent", "", "Target AI agent (e.g. claude-code)")
	aiAddCmd.Flags().Bool("global", false, "Install at user level instead of the current project")
	aiAddCmd.Flags().Bool("dry-run", false, "Show what would be installed without changing anything")
	aiAddCmd.Flags().BoolP("yes", "y", false, "Skip confirmation prompts")
	addFormatFlag(aiAddCmd)
}

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

		entry, ok := directory.Load().Get(name)
		if !ok {
			return fmt.Errorf("unknown integration %q (see `shopware-cli ai list`)", name)
		}

		// Skills only for now (MCP arrives later). The agent value is passed
		// straight to skills.sh.
		if entry.Type != directory.TypeSkill {
			return fmt.Errorf("installing %q is not supported yet (only skills for now)", name)
		}
		// A git skill's compatibility check is project-scoped, so a global
		// install of one is deferred to a later slice.
		if entry.Delivery.Kind == directory.DeliveryGit && global {
			return errors.New("global install of a git-delivered skill is not supported yet; install it into a project (omit --global)")
		}
		if agent == "" {
			return errors.New("specify the target client with --agent (e.g. --agent claude-code)")
		}

		// State lives where the config lives: a --global install and its state
		// go to the user config dir; a project install and its state go to the
		// current directory, matching where skills.sh writes the agent config.
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

		// Resolve the source repo and the ref to install. A bundled skill's
		// version follows the CLI; a git skill uses the explicit tag or the
		// latest stable release from its repository.
		var source, ref string
		switch entry.Delivery.Kind {
		case directory.DeliveryBundled:
			ref = tag
			if ref == "" {
				ref = cmd.Root().Version
			}
			source = "shopware/shopware-cli"
		case directory.DeliveryGit:
			ref = tag
			if ref == "" {
				if ref, err = resolveLatestTag(cmd.Context(), entry.Delivery.Repository); err != nil {
					return err
				}
			}
			source = ownerRepo(entry.Delivery.Repository)
		default:
			return fmt.Errorf("unsupported delivery %q", entry.Delivery.Kind)
		}
		if ref != "" {
			source += "@" + ref
		}

		// skills.sh must never prompt: this command already supplies the skill,
		// agent and scope, so its confirmation prompts are always skipped.
		argv := skillsAddArgs(source, entry.Name, agent, global, true)

		result := addResult{
			Name:             entry.Name,
			Client:           agent,
			Scope:            scope,
			RequestedTag:     tag,
			ResolvedRevision: ref,
			DryRun:           dryRun,
			Command:          argv,
		}

		if dryRun {
			return writeAddResult(cmd.OutOrStdout(), format, result)
		}

		current, err := readState()
		if err != nil {
			return err
		}

		// Idempotent: the same integration, client, scope and revision is a no-op.
		if !isInstalled(current, result) {
			if err := runSkills(cmd.Context(), argv, cmd.ErrOrStderr()); err != nil {
				return err
			}

			next := state.Upsert(current, state.InstalledEntry{
				Name:             result.Name,
				Client:           result.Client,
				Scope:            result.Scope,
				RequestedTag:     result.RequestedTag,
				ResolvedRevision: result.ResolvedRevision,
			})
			if err := saveState(next); err != nil {
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
	addFormatFlag(aiAddCmd)
}

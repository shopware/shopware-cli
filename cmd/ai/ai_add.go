package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/state"
	"github.com/shopware/shopware-cli/internal/shop"
)

// The CLI hardcodes no agent names: which agents exist is skills.sh's business,
// so a new one it supports works without a CLI change.

const (
	// bundledRepoURL is the repository a bundled skill is installed from; its ref
	// follows the CLI version.
	bundledRepoURL = "https://github.com/shopware/shopware-cli"
	// devVersion is the version a plain `go build` reports; it is not a real ref,
	// so bundled installs fall back to the latest release.
	devVersion = "dev"
	// refPlaceholder stands in for an unresolved ref in --dry-run output, so the
	// dry run stays offline (no tag lookup).
	refPlaceholder = "<latest-release>"
)

// Install outcomes reported by `ai add`.
const (
	actionInstalled = "installed"
	actionUnchanged = "unchanged"
	actionUpdated   = "updated"
)

// addResult is the machine-readable shape of `ai add` (--format json).
type addResult struct {
	Name             string      `json:"name"`
	Agent            string      `json:"agent"`
	Scope            state.Scope `json:"scope"`
	RequestedTag     string      `json:"requestedTag"`
	ResolvedRevision string      `json:"resolvedRevision"`
	Action           string      `json:"action,omitempty"`
	PreviousRevision string      `json:"previousRevision,omitempty"`
	DryRun           bool        `json:"dryRun"`
	Command          []string    `json:"command"`
}

var aiAddCmd = &cobra.Command{
	Use:          "add <name>[@<tag>]",
	Short:        "Install a Shopware AI integration into an AI agent",
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
		// A git skill's compatibility check runs against a project, so there is
		// nothing to check for a global install.
		if entry.Delivery.Kind == directory.DeliveryGit && global {
			return fmt.Errorf("%q must be installed into a project, not globally: it checks compatibility against that project (omit --global)", entry.Name)
		}
		if err := validateAgent(agent); err != nil {
			return err
		}

		scope := state.ScopeGlobal
		if !global {
			scope = state.ScopeProject
		}

		// Resolve the repo and the ref to install, then build a GitHub tree URL
		// that pins the ref (skills.sh ignores a bare owner/repo@ref). A bundled
		// skill follows the CLI version; a git skill uses the explicit tag or the
		// latest stable release. Tag lookups hit the network, so a --dry-run stays
		// offline and shows a placeholder when the ref cannot be resolved locally.
		var repoURL string
		ref := tag
		switch entry.Delivery.Kind {
		case directory.DeliveryBundled:
			repoURL = bundledRepoURL
			switch {
			case ref != "":
				if !dryRun {
					if err = verifyTag(cmd.Context(), repoURL, ref); err != nil {
						return err
					}
				}
			default:
				ref = cmd.Root().Version
				if ref == "" || ref == devVersion {
					if dryRun {
						ref = refPlaceholder
					} else if ref, err = resolveLatestTag(cmd.Context(), repoURL); err != nil {
						return err
					}
				}
			}
		case directory.DeliveryGit:
			repoURL = entry.Delivery.Repository
			switch {
			case ref == "":
				if dryRun {
					ref = refPlaceholder
				} else if ref, err = resolveLatestTag(cmd.Context(), repoURL); err != nil {
					return err
				}
			case !dryRun:
				if err = verifyTag(cmd.Context(), repoURL, ref); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unsupported delivery %q", entry.Delivery.Kind)
		}

		source := skillSourceURL(repoURL, ref, entry.Name)

		// skills.sh must never prompt: this command already supplies the source,
		// agent and scope, so its confirmation prompts are always skipped.
		argv := skillsAddArgs(source, agent, global, true)

		result := addResult{
			Name:             entry.Name,
			Agent:            agent,
			Scope:            scope,
			RequestedTag:     tag,
			ResolvedRevision: ref,
			DryRun:           dryRun,
			Command:          argv,
		}

		if dryRun {
			return writeAddResult(cmd.OutOrStdout(), format, result)
		}

		// State lives where the config lives: a --global install and its state go
		// to the user config dir; a project install resolves the Shopware project
		// root, so the state and the agent config land at the root (not in a
		// subdirectory), matching where other project commands operate.
		projectRoot := ""
		readState := state.Read
		saveState := state.Save
		if !global {
			root, err := shop.FindClosestShopwareProject(false)
			if err != nil {
				return fmt.Errorf("a project install must run inside a Shopware project (or use --global): %w", err)
			}
			projectRoot = root
			readState = func() (state.File, error) { return state.ReadProject(root) }
			saveState = func(f state.File) error { return state.SaveProject(root, f) }
		}

		current, err := readState()
		if err != nil {
			return err
		}

		// Record the prior revision (if any) to report the outcome as a fresh
		// install, a no-op, or an update.
		prev, hadPrev := findInstall(current, result.Name, result.Agent, result.Scope)

		// A git skill declares an owner-maintained compatibility check; run it
		// against the project before installing anything.
		if entry.Delivery.Kind == directory.DeliveryGit && entry.Compatibility != nil {
			if err := runCompatCheck(cmd.Context(), ownerRepo(entry.Delivery.Repository), entry.Name, ref, projectRoot, cmd.ErrOrStderr()); err != nil {
				return err
			}
		}

		// Always run skills.sh: it is idempotent and is the source of truth on
		// disk, so the record is never trusted over the actual installation (a
		// deleted skill is restored on a repeat add).
		if err := runSkills(cmd.Context(), argv, projectRoot, cmd.ErrOrStderr()); err != nil {
			return err
		}

		next := state.Upsert(current, state.InstalledEntry{
			Name:             result.Name,
			Agent:            result.Agent,
			Scope:            result.Scope,
			RequestedTag:     result.RequestedTag,
			ResolvedRevision: result.ResolvedRevision,
		})
		if err := saveState(next); err != nil {
			return fmt.Errorf("%s was installed but its record could not be written (re-run `ai add` to record it): %w", result.Name, err)
		}

		switch {
		case !hadPrev:
			result.Action = actionInstalled
		case prev.ResolvedRevision == result.ResolvedRevision:
			result.Action = actionUnchanged
		default:
			result.Action = actionUpdated
			result.PreviousRevision = prev.ResolvedRevision
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

// findInstall returns the recorded entry for (name, agent, scope) and whether one
// existed, so `ai add` can report a fresh install, a no-op, or an update.
func findInstall(f state.File, name, agent string, scope state.Scope) (state.InstalledEntry, bool) {
	for _, e := range f.Installed {
		if e.Name == name && e.Agent == agent && e.Scope == scope {
			return e, true
		}
	}

	return state.InstalledEntry{}, false
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
		_, err := fmt.Fprintf(w, "[dry-run] would install %s for %s (%s):\n  %s\n", r.Name, r.Agent, r.Scope, strings.Join(r.Command, " "))

		return err
	}

	rev := ""
	if r.ResolvedRevision != "" {
		rev = " @" + r.ResolvedRevision
	}

	switch r.Action {
	case actionUnchanged:
		_, err := fmt.Fprintf(w, "Already installed %s for %s (%s)%s\n", r.Name, r.Agent, r.Scope, rev)

		return err
	case actionUpdated:
		_, err := fmt.Fprintf(w, "Updated %s for %s (%s): %s → %s\n", r.Name, r.Agent, r.Scope, r.PreviousRevision, r.ResolvedRevision)

		return err
	default:
		_, err := fmt.Fprintf(w, "Installed %s for %s (%s)%s\n", r.Name, r.Agent, r.Scope, rev)

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

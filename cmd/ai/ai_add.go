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
	Use:   "add name[@tag]",
	Short: "Install a Shopware AI integration into an AI agent",
	Long: `Install a Shopware AI integration (a skill) into an AI agent via skills.sh.

By default the skill is installed into the current Shopware project; use --global
to install at the user level from anywhere. Pin a release with @<tag>, otherwise
the latest release (or the CLI version for bundled skills) is used.

Requirements: Node.js/npx on PATH; git-delivered integrations also need git and
network access. The agent name is whatever skills.sh supports (e.g. claude-code,
codex) — the CLI keeps no list of its own.

The install writes the agent's skill files plus skills-lock.json, and records the
install under .shopware-cli/ai/. Commit those to share the integration with your
team, or gitignore them to keep it local.`,
	Example: `  # install the Shopware CLI skill into Claude Code for this project
  shopware-cli ai add shopware-cli --agent claude-code

  # pin a specific release
  shopware-cli ai add deployment-helper@0.1.7 --agent claude-code

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

		result, err := performAdd(cmd.Context(), addOptions{
			name:       args[0],
			agent:      agent,
			global:     global,
			dryRun:     dryRun,
			cliVersion: cmd.Root().Version,
		}, cmd.ErrOrStderr())
		if err != nil {
			return err
		}

		return writeAddResult(cmd.OutOrStdout(), format, result)
	},
}

// addOptions are the inputs to performAdd, parsed from the command flags.
type addOptions struct {
	name       string // may carry a @tag suffix
	agent      string
	global     bool
	dryRun     bool
	cliVersion string
}

// performAdd runs the `ai add` flow and returns the outcome. It resolves the
// pinned source, (for a git skill) runs the compatibility check, delegates the
// install to skills.sh and records it; skills.sh output and check messages go to
// progress. It lives outside the cobra command so the behaviour can be tested
// directly.
func performAdd(ctx context.Context, o addOptions, progress io.Writer) (addResult, error) {
	name, tag := splitNameTag(o.name)

	entry, ok := directory.Load().Get(name)
	if !ok {
		return addResult{}, fmt.Errorf("unknown integration %q (see `shopware-cli ai list`)", name)
	}

	// Skills only for now (MCP arrives later). The agent value is passed
	// straight to skills.sh.
	if entry.Type != directory.TypeSkill {
		return addResult{}, fmt.Errorf("installing %q is not supported yet (only skills for now)", name)
	}
	// A git skill's compatibility check runs against a project, so there is
	// nothing to check for a global install.
	if entry.Delivery.Kind == directory.DeliveryGit && o.global {
		return addResult{}, fmt.Errorf("%q must be installed into a project, not globally: it checks compatibility against that project (omit --global)", entry.Name)
	}
	if err := validateAgent(o.agent); err != nil {
		return addResult{}, err
	}

	scope := state.ScopeGlobal
	if !o.global {
		scope = state.ScopeProject
	}

	repoURL, ref, err := resolveSource(ctx, entry, tag, o.cliVersion, o.dryRun)
	if err != nil {
		return addResult{}, err
	}

	// skills.sh must never prompt: this command already supplies the source,
	// agent and scope, so its confirmation prompts are always skipped.
	argv := skillsAddArgs(skillSourceURL(repoURL, ref, entry.Name), o.agent, o.global, true)

	result := addResult{
		Name:             entry.Name,
		Agent:            o.agent,
		Scope:            scope,
		RequestedTag:     tag,
		ResolvedRevision: ref,
		DryRun:           o.dryRun,
		Command:          argv,
	}

	if o.dryRun {
		return result, nil
	}

	// State lives where the config lives: a --global install and its state go to
	// the user config dir; a project install resolves the Shopware project root,
	// so the state and the agent config land at the root (not in a subdirectory),
	// matching where other project commands operate.
	projectRoot := ""
	readState := state.Read
	saveState := state.Save
	if !o.global {
		root, err := shop.FindClosestShopwareProject(false)
		if err != nil {
			return addResult{}, fmt.Errorf("a project install must run inside a Shopware project (or use --global): %w", err)
		}
		projectRoot = root
		readState = func() (state.File, error) { return state.ReadProject(root) }
		saveState = func(f state.File) error { return state.SaveProject(root, f) }
	}

	current, err := readState()
	if err != nil {
		return addResult{}, err
	}

	// Record the prior revision (if any) to report the outcome as a fresh
	// install, a no-op, or an update.
	prev, hadPrev := findInstall(current, result.Name, result.Agent, result.Scope)

	// A git skill declares an owner-maintained compatibility check; run it
	// against the project before installing anything.
	if entry.Delivery.Kind == directory.DeliveryGit && entry.Compatibility != nil {
		if err := runCompatCheck(ctx, ownerRepo(entry.Delivery.Repository), entry.Name, ref, projectRoot, progress); err != nil {
			return addResult{}, err
		}
	}

	// Always run skills.sh: it is idempotent and is the source of truth on disk,
	// so the record is never trusted over the actual installation (a deleted
	// skill is restored on a repeat add).
	if err := runSkills(ctx, argv, projectRoot, progress); err != nil {
		return addResult{}, err
	}

	next := state.Upsert(current, state.InstalledEntry{
		Name:             result.Name,
		Agent:            result.Agent,
		Scope:            result.Scope,
		RequestedTag:     result.RequestedTag,
		ResolvedRevision: result.ResolvedRevision,
	})
	if err := saveState(next); err != nil {
		return addResult{}, fmt.Errorf("%s was installed but its record could not be written (re-run `ai add` to record it): %w", result.Name, err)
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

	return result, nil
}

// resolveSource resolves the repo URL and the git ref to install for entry; the
// caller pins them into a GitHub tree URL (skills.sh ignores a bare
// owner/repo@ref). A bundled skill follows the CLI version; a git skill uses the
// explicit tag or the latest stable release. Tag lookups hit the network, so a
// --dry-run stays offline and returns a placeholder ref instead.
func resolveSource(ctx context.Context, entry *directory.Integration, tag, cliVersion string, dryRun bool) (repoURL, ref string, err error) {
	ref = tag
	switch entry.Delivery.Kind {
	case directory.DeliveryBundled:
		repoURL = bundledRepoURL
		switch {
		case ref != "":
			if !dryRun {
				err = verifyTag(ctx, repoURL, ref)
			}
		case dryRun && (cliVersion == "" || cliVersion == devVersion):
			ref = refPlaceholder
		case cliVersion == "" || cliVersion == devVersion:
			ref, err = resolveLatestTag(ctx, repoURL)
		default:
			ref = cliVersion
		}
	case directory.DeliveryGit:
		repoURL = entry.Delivery.Repository
		switch {
		case ref == "" && dryRun:
			ref = refPlaceholder
		case ref == "":
			ref, err = resolveLatestTag(ctx, repoURL)
		case !dryRun:
			err = verifyTag(ctx, repoURL, ref)
		}
	default:
		err = fmt.Errorf("unsupported delivery %q", entry.Delivery.Kind)
	}

	return repoURL, ref, err
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

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/skills"
	"github.com/shopware/shopware-cli/internal/ai/state"
	"github.com/shopware/shopware-cli/internal/shop"
)

const (
	// bundledRepoURL is where bundled skills are installed from.
	bundledRepoURL = "https://github.com/shopware/shopware-cli"
	// devVersion is what a plain `go build` reports (not a real ref).
	devVersion = "dev"
	// refPlaceholder is shown for an unresolved ref in --dry-run.
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
	Dir              string      `json:"dir,omitempty"`
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

By default the skill is installed into the current Shopware project (a shop); use
--global to install at the user level from anywhere (the right choice in an
extension repository, which is not a shop). Pin a release with @<tag>, otherwise
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

// performAdd resolves the pinned source, runs any compatibility check, installs
// via skills.sh and records the result. skills.sh output goes to progress.
func performAdd(ctx context.Context, o addOptions, progress io.Writer) (addResult, error) {
	name, tag := splitNameTag(o.name)

	entry, ok := directory.Load().Get(name)
	if !ok {
		return addResult{}, fmt.Errorf("unknown integration %q (see \"shopware-cli ai list\")", name)
	}

	// Skills only for now (MCP later).
	if entry.Type != directory.TypeSkill {
		return addResult{}, fmt.Errorf("installing %q is not supported yet (only skills for now)", name)
	}
	// A git skill's compatibility check needs a project, so no global install.
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

	// Project scope resolves the Shopware project root (filesystem only, so a
	// dry-run resolves it too); global uses the user config dir.
	projectRoot := ""
	if !o.global {
		root, err := shop.FindClosestShopwareProject(false)
		if err != nil {
			return addResult{}, fmt.Errorf("a project install must run inside a Shopware project (or use --global): %w", err)
		}
		projectRoot = root
	}

	repoURL, ref, err := resolveSource(ctx, entry, tag, o.cliVersion, o.dryRun)
	if err != nil {
		return addResult{}, err
	}

	// -y: we supply everything, so skills.sh must not prompt.
	argv := skills.AddArgs(skills.SourceURL(repoURL, ref, entry.Name), o.agent, o.global, true)

	result := addResult{
		Name:             entry.Name,
		Agent:            o.agent,
		Scope:            scope,
		Dir:              projectRoot,
		RequestedTag:     tag,
		ResolvedRevision: ref,
		DryRun:           o.dryRun,
		Command:          argv,
	}

	if o.dryRun {
		return result, nil
	}

	readState := state.Read
	saveState := state.Save
	if !o.global {
		readState = func() (state.File, error) { return state.ReadProject(projectRoot) }
		saveState = func(f state.File) error { return state.SaveProject(projectRoot, f) }
	}

	current, err := readState()
	if err != nil {
		return addResult{}, err
	}

	// Prior revision drives the installed/unchanged/updated outcome.
	prev, hadPrev := findInstall(current, result.Name, result.Agent, result.Scope)

	// Check compatibility before installing anything.
	if entry.Delivery.Kind == directory.DeliveryGit && entry.Compatibility != nil {
		if err := skills.RunCompatCheck(ctx, skills.OwnerRepo(entry.Delivery.Repository), entry.Name, ref, projectRoot, progress); err != nil {
			return addResult{}, err
		}
	}

	// Always run skills.sh (idempotent); it owns the disk, the record only reports.
	if err := skills.Run(ctx, argv, projectRoot, progress); err != nil {
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
		return addResult{}, fmt.Errorf("%s was installed but its record cannot be written, re-run \"ai add\": %w", result.Name, err)
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

// resolveSource returns the repo URL and ref to install: a bundled skill follows
// the CLI version, a git skill the explicit tag or latest release. A dry-run
// skips the network lookup and returns a placeholder ref.
func resolveSource(ctx context.Context, entry *directory.Integration, tag, cliVersion string, dryRun bool) (repoURL, ref string, err error) {
	ref = tag
	switch entry.Delivery.Kind {
	case directory.DeliveryBundled:
		repoURL = bundledRepoURL
		switch {
		case ref != "":
			if !dryRun {
				ref, err = skills.ResolveTag(ctx, repoURL, ref)
			}
		case cliVersion == "" || cliVersion == devVersion:
			if dryRun {
				ref = refPlaceholder
			} else {
				ref, err = skills.ResolveLatestTag(ctx, repoURL)
			}
		case !dryRun:
			ref, err = skills.ResolveTag(ctx, repoURL, cliVersion)
		default:
			ref = cliVersion
		}
	case directory.DeliveryGit:
		repoURL = entry.Delivery.Repository
		switch {
		case ref == "":
			if dryRun {
				ref = refPlaceholder
			} else {
				ref, err = skills.ResolveLatestTag(ctx, repoURL)
			}
		case !dryRun:
			ref, err = skills.ResolveTag(ctx, repoURL, ref)
		}
	default:
		err = fmt.Errorf("unsupported delivery %q", entry.Delivery.Kind)
	}

	return repoURL, ref, err
}

// validateAgent rejects --agent shapes skills.sh cannot round-trip: empty, the
// "*" wildcard, and comma/whitespace lists. The name itself is not checked.
func validateAgent(agent string) error {
	if agent == "" {
		return errors.New("specify the target agent with --agent (e.g. --agent claude-code)")
	}
	if strings.ContainsAny(agent, "*, \t") {
		return fmt.Errorf("--agent takes a single agent (e.g. claude-code); %q is not supported, run the command once per agent", agent)
	}

	return nil
}

// splitNameTag splits "name@tag" into its parts; a missing tag yields "".
func splitNameTag(s string) (name, tag string) {
	if i := strings.IndexByte(s, '@'); i >= 0 {
		return s[:i], s[i+1:]
	}

	return s, ""
}

// findInstall returns the recorded entry for (name, agent, scope), if any.
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
	case actionUnchanged:
		_, err := fmt.Fprintf(w, "Already installed %s for %s (%s)%s%s\n", r.Name, r.Agent, r.Scope, dest, rev)

		return err
	case actionUpdated:
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

// Package install orchestrates installing and removing Shopware AI integrations:
// it resolves the pinned source, runs skills.sh (via the skills package) and
// records the result in the install state. The cmd/ai layer stays a thin front
// end that only parses flags and renders the returned result.
package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/skills"
	"github.com/shopware/shopware-cli/internal/ai/state"
	"github.com/shopware/shopware-cli/internal/cliversion"
	"github.com/shopware/shopware-cli/internal/shop"
)

// bundledRepoURL is where bundled skills are installed from.
const bundledRepoURL = "https://github.com/shopware/shopware-cli"

// devVersion is what a plain `go build` reports (not a real ref).
const devVersion = "dev"

// Install outcomes reported by Add.
const (
	ActionInstalled = "installed"
	ActionUnchanged = "unchanged"
	ActionUpdated   = "updated"
)

// runSkills is a seam over skills.Run so tests can stub the npx call.
var runSkills = skills.Run

// Options are the inputs to Add.
type Options struct {
	Name   string // may carry a @tag suffix
	Agent  string
	Global bool
	DryRun bool
}

// Result is the machine-readable outcome of Add (the `ai add` JSON contract).
type Result struct {
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

// Add installs a skill into an agent via skills.sh and records it. skills.sh
// output is streamed to progress.
func Add(ctx context.Context, o Options, progress io.Writer) (Result, error) {
	name, tag := splitNameTag(o.Name)

	entry, ok := directory.Load().Get(name)
	if !ok {
		return Result{}, fmt.Errorf("unknown integration %q (see \"shopware-cli ai list\")", name)
	}
	if entry.Type != directory.TypeSkill {
		return Result{}, fmt.Errorf("installing %q is not supported", entry.Name)
	}
	if err := validateAgent(o.Agent); err != nil {
		return Result{}, err
	}

	st := store{global: o.Global}
	scope := state.ScopeGlobal
	if !o.Global {
		root, err := shop.FindClosestShopwareProject(false)
		if err != nil {
			return Result{}, fmt.Errorf("a project install must run inside a Shopware project (or use --global): %w", err)
		}
		st.root = root
		scope = state.ScopeProject
	}

	repoURL, ref := resolveSource(entry, tag)

	result := Result{
		Name:             entry.Name,
		Agent:            o.Agent,
		Scope:            scope,
		Dir:              st.root,
		RequestedTag:     tag,
		ResolvedRevision: ref,
		DryRun:           o.DryRun,
		Command:          skills.AddArgs(skills.Source(repoURL, ref, entry.Name), entry.Name, o.Agent, o.Global, true),
	}
	if o.DryRun {
		return result, nil
	}

	current, err := st.read()
	if err != nil {
		return Result{}, err
	}

	// Prior revision drives the installed/unchanged/updated outcome.
	prev, hadPrev := findInstall(current, result.Name, result.Agent, result.Scope)

	// skills.sh owns the disk and is idempotent; the record only reports.
	if err := runSkills(ctx, result.Command, st.root, progress); err != nil {
		return Result{}, err
	}

	next := state.Upsert(current, state.InstalledEntry{
		Name:             result.Name,
		Agent:            result.Agent,
		Scope:            result.Scope,
		RequestedTag:     result.RequestedTag,
		ResolvedRevision: result.ResolvedRevision,
	})
	if err := st.save(next); err != nil {
		return Result{}, fmt.Errorf("%s was installed but its record cannot be written, re-run \"ai add\": %w", result.Name, err)
	}

	switch {
	case !hadPrev:
		result.Action = ActionInstalled
	case prev.ResolvedRevision == result.ResolvedRevision:
		result.Action = ActionUnchanged
	default:
		result.Action = ActionUpdated
		result.PreviousRevision = prev.ResolvedRevision
	}

	return result, nil
}

// RemoveOptions are the inputs to Remove.
type RemoveOptions struct {
	Name   string
	Agent  string
	Global bool
	DryRun bool
}

// RemoveResult is the machine-readable outcome of Remove.
type RemoveResult struct {
	Name    string      `json:"name"`
	Agent   string      `json:"agent"`
	Scope   state.Scope `json:"scope"`
	Removed bool        `json:"removed"`
	DryRun  bool        `json:"dryRun"`
	Command []string    `json:"command"`
}

// Remove uninstalls via skills.sh and drops the record, but only for what the CLI
// recorded; a hand-written agent config is left alone.
func Remove(ctx context.Context, o RemoveOptions, progress io.Writer) (RemoveResult, error) {
	if err := validateAgent(o.Agent); err != nil {
		return RemoveResult{}, err
	}

	// A recorded install is enough to remove, even if it left the directory.
	entry, known := directory.Load().Get(o.Name)
	removeName := o.Name
	if known {
		removeName = entry.Name
	}

	st := store{global: o.Global}
	scope := state.ScopeGlobal
	if !o.Global {
		root, err := shop.FindClosestShopwareProject(false)
		if err != nil {
			return RemoveResult{}, fmt.Errorf("a project removal must run inside a Shopware project (or use --global): %w", err)
		}
		st.root = root
		scope = state.ScopeProject
	}

	current, err := st.read()
	if err != nil {
		return RemoveResult{}, err
	}

	next, recorded := state.Remove(current, removeName, o.Agent, scope)

	// Unknown name with nothing recorded is a typo, not a stale install.
	if !recorded && !known {
		return RemoveResult{}, fmt.Errorf("unknown integration %q (see \"shopware-cli ai list\")", o.Name)
	}

	result := RemoveResult{
		Name:    removeName,
		Agent:   o.Agent,
		Scope:   scope,
		Removed: recorded,
		DryRun:  o.DryRun,
		Command: skills.RemoveArgs(removeName, o.Agent, o.Global),
	}
	if o.DryRun || !recorded {
		return result, nil
	}

	if err := runSkills(ctx, result.Command, st.root, progress); err != nil {
		return RemoveResult{}, err
	}

	// Remove the state file once empty instead of leaving an empty one.
	if len(next.Installed) == 0 {
		err = st.clear()
	} else {
		err = st.save(next)
	}
	if err != nil {
		return RemoveResult{}, fmt.Errorf("%s was removed but its record cannot be updated, re-run \"ai remove\": %w", result.Name, err)
	}

	return result, nil
}

// resolveSource returns the repo URL and ref to install. A ref is derived without
// a network call: an explicit tag (or the CLI version for bundled skills),
// normalized to the v-prefixed tag form. An empty ref installs the default branch.
func resolveSource(entry *directory.Integration, tag string) (repoURL, ref string) {
	switch entry.Delivery.Kind {
	case directory.DeliveryBundled:
		repoURL = bundledRepoURL
		switch {
		case tag != "":
			ref = skills.NormalizeRef(tag)
		case cliversion.Version != "" && cliversion.Version != devVersion:
			ref = skills.NormalizeRef(cliversion.Version)
		}
	case directory.DeliveryGit:
		repoURL = entry.Delivery.Repository
		if tag != "" {
			ref = skills.NormalizeRef(tag)
		}
	}

	return repoURL, ref
}

// store reads and writes the install state at the right scope: the user config
// dir for a global install, the project root for a project install.
type store struct {
	global bool
	root   string
}

func (s store) read() (state.File, error) {
	if s.global {
		return state.Read()
	}

	return state.ReadProject(s.root)
}

func (s store) save(f state.File) error {
	if s.global {
		return state.Save(f)
	}

	return state.SaveProject(s.root, f)
}

func (s store) clear() error {
	if s.global {
		return state.Clear()
	}

	return state.ClearProject(s.root)
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

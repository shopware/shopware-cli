// Package install orchestrates installing and removing Shopware AI integrations:
// it runs skills.sh (via the skills package) in the right scope. skills.sh is the
// source of truth for what is installed, so the CLI keeps no record of its own.
// The cmd/ai layer stays a thin front end that parses flags and renders the result.
package install

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/shopware/shopware-cli/internal/ai/directory"
	"github.com/shopware/shopware-cli/internal/ai/mcp"
	"github.com/shopware/shopware-cli/internal/ai/skills"
	"github.com/shopware/shopware-cli/internal/shop"
)

// Install scopes.
const (
	ScopeProject = "project"
	ScopeGlobal  = "global"
)

// Seams over the skills package so tests can stub the npx calls.
var (
	runSkills  = skills.Run
	listSkills = skills.List
)

// Options are the inputs to Add.
type Options struct {
	Name   string // may carry a @tag suffix
	Agent  string
	Global bool
	DryRun bool
}

// AddResult is the outcome of Add, rendered by the command as a human-readable line.
// On a post-resolution failure (the install step itself fails) the identifying
// fields are still populated alongside the error, so the caller can report what
// was attempted; on an input or guard error the zero AddResult is returned.
type AddResult struct {
	Name             string
	Type             directory.Type
	Agent            string
	Scope            string
	Dir              string
	RequestedTag     string
	ResolvedRevision string
	DryRun           bool
	Command          []string
}

// Add installs a skill into an agent via skills.sh. skills.sh output is streamed
// to progress and it owns the on-disk record (skills-lock.json).
func Add(ctx context.Context, o Options, progress io.Writer) (AddResult, error) {
	name, tag := splitNameTag(o.Name)
	if tag == "" && strings.Contains(o.Name, "@") {
		return AddResult{}, fmt.Errorf("empty version after \"@\" in %q; pin a release like %s@v1.2.3 or omit the @", o.Name, name)
	}

	entry, ok := directory.Load().Get(name)
	if !ok {
		return AddResult{}, fmt.Errorf("unknown integration %q (see \"shopware-cli ai list\")", name)
	}
	if entry.Type == directory.TypeMCP {
		return addMCP(o, entry, tag)
	}
	if entry.Type != directory.TypeSkill {
		return AddResult{}, fmt.Errorf("installing %q is not supported", entry.Name)
	}
	if err := validateAgent(o.Agent); err != nil {
		return AddResult{}, err
	}

	scope, dir, err := resolveScope(o.Global)
	if err != nil {
		return AddResult{}, err
	}

	// The tag is the git ref verbatim (e.g. v0.1.9); an empty tag installs the
	// repository's default branch.
	result := AddResult{
		Name:             entry.Name,
		Type:             entry.Type,
		Agent:            o.Agent,
		Scope:            scope,
		Dir:              dir,
		RequestedTag:     tag,
		ResolvedRevision: tag,
		DryRun:           o.DryRun,
		Command:          skills.AddArgs(skills.Source(entry.Delivery.Repository, tag, entry.Name), entry.Name, o.Agent, o.Global, true),
	}
	if o.DryRun {
		return result, nil
	}

	if err := runSkills(ctx, result.Command, dir, progress); err != nil {
		return result, err
	}

	return result, nil
}

// addMCP installs an MCP server by writing the project's .mcp.json — the standard
// file MCP clients read — rather than any agent's own config. It is project-scoped
// (committing the file shares the server with the team) and takes no @tag; the
// --agent flag does not apply because the file is not agent-specific.
func addMCP(o Options, entry *directory.Integration, tag string) (AddResult, error) {
	if tag != "" {
		return AddResult{}, fmt.Errorf("%q is an MCP server and cannot be pinned with @tag", entry.Name)
	}

	root, err := mcpProjectRoot(o.Agent, o.Global)
	if err != nil {
		return AddResult{}, err
	}
	path := mcp.ConfigPath(root)

	result := AddResult{
		Name:    entry.Name,
		Type:    entry.Type,
		Scope:   ScopeProject,
		Dir:     root,
		DryRun:  o.DryRun,
		Command: []string{"writes", path},
	}
	if o.DryRun {
		return result, nil
	}

	if err := mcp.Add(root, entry.Name, entry.Delivery.URL, entry.Delivery.Transport); err != nil {
		return result, err
	}

	return result, nil
}

// mcpProjectRoot resolves the project whose .mcp.json an MCP install writes. MCP
// servers live with the project and are not agent-specific, so --agent and
// --global are rejected rather than silently ignored (either would otherwise let
// an agent- or user-scoped request change the shared project config).
func mcpProjectRoot(agent string, global bool) (string, error) {
	if agent != "" {
		return "", errors.New("--agent does not apply to an MCP server; it is written to the project's .mcp.json for all agents")
	}
	if global {
		return "", errors.New("an MCP server is written to the project's .mcp.json; --global is not supported")
	}

	root, err := shop.FindClosestShopwareProject(false)
	if err != nil {
		return "", fmt.Errorf("an MCP install must run inside a Shopware project: %w", err)
	}

	return root, nil
}

// RemoveOptions are the inputs to Remove.
type RemoveOptions struct {
	Name   string
	Agent  string
	Global bool
	DryRun bool
}

// RemoveResult is the outcome of Remove, rendered by the command as a line.
// Removed is false when the skill was not installed for that agent and scope, so
// skills.sh had nothing to delete. As with AddResult, a post-resolution failure
// still populates the identifying fields alongside the error.
type RemoveResult struct {
	Name    string
	Type    directory.Type
	Agent   string
	Scope   string
	Removed bool
	DryRun  bool
	Command []string
}

// Remove uninstalls a skill from an agent via skills.sh. It first asks skills.sh
// whether the skill is installed for that agent and scope, because skills.sh
// reports success even when there was nothing to remove.
func Remove(ctx context.Context, o RemoveOptions, progress io.Writer) (RemoveResult, error) {
	entry, ok := directory.Load().Get(o.Name)
	if !ok {
		return RemoveResult{}, fmt.Errorf("unknown integration %q (see \"shopware-cli ai list\")", o.Name)
	}
	if entry.Type == directory.TypeMCP {
		return removeMCP(o, entry)
	}

	if err := validateAgent(o.Agent); err != nil {
		return RemoveResult{}, err
	}

	scope, dir, err := resolveScope(o.Global)
	if err != nil {
		return RemoveResult{}, err
	}

	result := RemoveResult{
		Name:    entry.Name,
		Type:    entry.Type,
		Agent:   o.Agent,
		Scope:   scope,
		DryRun:  o.DryRun,
		Command: skills.RemoveArgs(entry.Name, o.Agent, o.Global),
	}

	installed, err := installedForAgent(ctx, dir, o.Global, entry.Name, o.Agent)
	if err != nil {
		return result, err
	}
	result.Removed = installed

	if o.DryRun || !installed {
		return result, nil
	}

	if err := runSkills(ctx, result.Command, dir, progress); err != nil {
		return result, err
	}

	return result, nil
}

// removeMCP removes an MCP server from the project's .mcp.json, deleting exactly
// the named entry and reporting whether it was there.
func removeMCP(o RemoveOptions, entry *directory.Integration) (RemoveResult, error) {
	root, err := mcpProjectRoot(o.Agent, o.Global)
	if err != nil {
		return RemoveResult{}, err
	}
	path := mcp.ConfigPath(root)

	result := RemoveResult{
		Name:    entry.Name,
		Type:    entry.Type,
		Scope:   ScopeProject,
		DryRun:  o.DryRun,
		Command: []string{"updates", path},
	}
	if o.DryRun {
		present, err := mcp.Has(root, entry.Name)
		if err != nil {
			return result, err
		}
		result.Removed = present

		return result, nil
	}

	removed, err := mcp.Remove(root, entry.Name)
	if err != nil {
		return result, err
	}
	result.Removed = removed

	return result, nil
}

// installedForAgent reports whether skills.sh has the named skill installed for
// the given agent at the given scope.
func installedForAgent(ctx context.Context, dir string, global bool, name, agent string) (bool, error) {
	records, err := listSkills(ctx, dir, global)
	if err != nil {
		return false, err
	}

	for _, r := range records {
		if r.Name == name && hasAgent(r.Agents, agent) {
			return true, nil
		}
	}

	return false, nil
}

// hasAgent reports whether target (an agent id like "claude-code") is among the
// agent display names skills.sh returns (e.g. "Claude Code"), comparing on
// alphanumerics only so ids and display names match.
func hasAgent(agents []string, target string) bool {
	want := normalizeAgent(target)
	for _, a := range agents {
		if normalizeAgent(a) == want {
			return true
		}
	}

	return false
}

// nonAlnum matches everything that is not a lowercase letter or digit.
var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// normalizeAgent canonicalizes an agent id or display name ("Claude Code",
// "claude-code") to a comparable token ("claudecode").
func normalizeAgent(s string) string {
	return nonAlnum.ReplaceAllString(strings.ToLower(s), "")
}

// resolveScope returns the scope label and the directory to run skills.sh in. A
// project install resolves the Shopware project root so skills.sh writes the agent
// config and skills-lock.json there; a global install runs anywhere.
func resolveScope(global bool) (scope, dir string, err error) {
	if global {
		return ScopeGlobal, "", nil
	}

	root, err := shop.FindClosestShopwareProject(false)
	if err != nil {
		return "", "", fmt.Errorf("a project install must run inside a Shopware project (or use --global): %w", err)
	}

	return ScopeProject, root, nil
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

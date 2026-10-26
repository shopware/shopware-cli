// Package skills is a thin client over the skills.sh CLI (via npx): it builds the
// argv to install and remove skills in AI agents, and the GitHub source that pins
// a skill to a ref. skills.sh does the cloning and writes the agent config.
package skills

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"time"
)

// skillsVersion pins the skills.sh major line; npx resolves the latest 1.x, so we
// get fixes but never a breaking 2.0.
const skillsVersion = "1"

// AddArgs builds the `npx skills add <source> --skill <skill> ...` argv. --skill
// selects the one skill whether the source is a ref-pinned tree URL or a bare
// owner/repo (its default branch).
func AddArgs(source, skill, agent string, global, assumeYes bool) []string {
	argv := []string{
		"npx", "--yes", "skills@" + skillsVersion, "add", source,
		"--skill", skill,
		"--agent", agent,
	}
	if global {
		argv = append(argv, "--global")
	}
	if assumeYes {
		argv = append(argv, "-y")
	}

	return argv
}

// RemoveArgs builds the `npx skills remove ...` argv.
func RemoveArgs(skill, agent string, global bool) []string {
	argv := []string{
		"npx", "--yes", "skills@" + skillsVersion, "remove", skill,
		"--agent", agent,
	}
	if global {
		argv = append(argv, "--global")
	}
	argv = append(argv, "-y")

	return argv
}

// Source returns the skills.sh source argument for a GitHub "owner/repo": a tree
// URL pinned to ref (e.g. shopware/shopware-cli/tree/v0.18.3/skills/shopware-cli),
// or the bare owner/repo (its default branch) when ref is empty.
func Source(ownerRepo, ref, skill string) string {
	if ref == "" {
		return ownerRepo
	}

	return fmt.Sprintf("https://github.com/%s/tree/%s/skills/%s", ownerRepo, ref, skill)
}

// Run runs a skills.sh command via npx in dir (empty = current directory),
// streaming output to out.
func Run(ctx context.Context, argv []string, dir string, out io.Writer) error {
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("%s not found: installing skills requires Node.js/npx on PATH", argv[0])
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("skills %s failed: %w", op(argv), err)
	}

	return nil
}

// op returns the skills.sh subcommand from argv, for error messages.
func op(argv []string) string {
	if len(argv) > 3 {
		return argv[3]
	}

	return "command"
}

// Installed is one entry of `skills list --json`: skills.sh is the source of
// truth for what is installed, so the CLI reads it instead of keeping its own
// record.
type Installed struct {
	Name   string   `json:"name"`
	Scope  string   `json:"scope"`
	Agents []string `json:"agents"`
	Source string   `json:"source"`
}

// List returns the skills skills.sh has installed at the given scope, parsed from
// `skills list --json`. dir is where to run it (the project root for project
// scope); it is ignored for the global scope.
func List(ctx context.Context, dir string, global bool) ([]Installed, error) {
	argv := []string{"npx", "--yes", "skills@" + skillsVersion, "list", "--json"}
	if global {
		argv = append(argv, "--global")
	}

	if _, err := exec.LookPath(argv[0]); err != nil {
		return nil, fmt.Errorf("%s not found: listing skills requires Node.js/npx on PATH", argv[0])
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	// When ctx is cancelled (e.g. the recommendation's lookup timeout), a spawned
	// skills child can keep the stdout pipe open; WaitDelay bounds how long Run
	// waits for it instead of blocking indefinitely.
	cmd.WaitDelay = 2 * time.Second
	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("skills list failed: %w", err)
	}

	var out []Installed
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("cannot parse skills list output: %w", err)
	}

	return out, nil
}

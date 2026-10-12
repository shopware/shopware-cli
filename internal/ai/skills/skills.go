// Package skills is a thin client over the skills.sh CLI (via npx): it builds the
// argv to install and remove skills in AI agents, and the GitHub source that pins
// a skill to a ref. skills.sh does the cloning and writes the agent config.
package skills

import (
	"context"
	"fmt"
	"io"
	"os/exec"
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

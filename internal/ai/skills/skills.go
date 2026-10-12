// Package skills is a thin client over the skills.sh CLI (via npx): it builds the
// argv to install and remove skills in AI agents, and the GitHub source that pins
// a skill to a ref. skills.sh does the cloning and writes the agent config.
package skills

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
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

// Source returns the skills.sh source argument: a GitHub tree URL pinned to ref
// (e.g. .../shopware-cli/tree/v0.18.3/skills/shopware-cli), or the bare owner/repo
// (its default branch) when ref is empty.
func Source(repoURL, ref, skill string) string {
	if ref == "" {
		return OwnerRepo(repoURL)
	}

	return fmt.Sprintf("https://github.com/%s/tree/%s/skills/%s", OwnerRepo(repoURL), ref, skill)
}

// OwnerRepo turns a GitHub repo URL into "owner/repo" form.
func OwnerRepo(repoURL string) string {
	s := strings.TrimSuffix(repoURL, ".git")
	s = strings.TrimPrefix(s, "https://github.com/")
	s = strings.TrimPrefix(s, "http://github.com/")

	return s
}

// NormalizeRef maps a bare version like "0.1.9" to the "v"-prefixed tag form the
// Shopware repositories now use ("v0.1.9"). A ref that already carries the "v" or
// is not a bare version (a branch name, a commit) is returned unchanged, so a ref
// can be resolved without a network lookup.
func NormalizeRef(ref string) string {
	if ref != "" && ref[0] >= '0' && ref[0] <= '9' {
		return "v" + ref
	}

	return ref
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

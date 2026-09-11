package ai

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/shyim/go-version"
)

// skillsVersion pins the skills.sh CLI the commands run against, so a
// shopware-cli release always drives a known skills.sh behavior.
const skillsVersion = "1.5.18"

// skillsAddArgs builds the `npx skills add ...` argv. shopware-cli is the front
// door: it decides what to install; skills.sh writes the agent configuration.
// `ai add --dry-run` prints this without running it.
func skillsAddArgs(source, skill, agent string, global, assumeYes bool) []string {
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

// skillsRemoveArgs builds the `npx skills remove ...` argv. skills.sh never
// prompts here: the skill, agent and scope are all supplied.
func skillsRemoveArgs(skill, agent string, global bool) []string {
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

// runSkills runs a skills.sh command via npx, streaming its output to out so
// the user sees exactly what skills.sh did (which files it wrote, where). It is
// a package var so tests can substitute it without shelling out. Callers pass
// stderr as out to keep stdout clean for --format json.
var runSkills = func(ctx context.Context, argv []string, out io.Writer) error {
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("%s not found: installing skills requires Node.js/npx on PATH", argv[0])
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("skills %s failed: %w", skillsOp(argv), err)
	}

	return nil
}

// skillsOp returns the skills.sh subcommand from an argv built by
// skillsAddArgs/skillsRemoveArgs (npx --yes skills@<ver> <op> …), for error
// messages. It falls back to "command" for an unexpected shape.
func skillsOp(argv []string) string {
	if len(argv) > 3 {
		return argv[3]
	}

	return "command"
}

// ownerRepo turns a GitHub repository URL into the "owner/repo" form skills.sh
// expects (https://github.com/shopware/deployment-helper -> shopware/deployment-helper).
func ownerRepo(repoURL string) string {
	s := strings.TrimSuffix(repoURL, ".git")
	s = strings.TrimPrefix(s, "https://github.com/")
	s = strings.TrimPrefix(s, "http://github.com/")

	return s
}

// resolveLatestTag returns the highest stable release tag of repoURL, read with
// `git ls-remote --tags`. It is a package var so tests can substitute it without
// network access.
var resolveLatestTag = func(ctx context.Context, repoURL string) (string, error) {
	out, err := exec.CommandContext(ctx, "git", "ls-remote", "--tags", repoURL).Output()
	if err != nil {
		return "", fmt.Errorf("cannot list release tags of %s: %w", repoURL, err)
	}

	tag, err := latestStableTag(strings.Split(string(out), "\n"))
	if err != nil {
		return "", fmt.Errorf("%s: %w", repoURL, err)
	}

	return tag, nil
}

// latestStableTag picks the highest stable semver tag from `git ls-remote --tags`
// output lines ("<sha>\trefs/tags/<tag>"). Pre-releases are ignored.
func latestStableTag(lsRemoteLines []string) (string, error) {
	var best *version.Version
	var bestRaw string

	for _, line := range lsRemoteLines {
		i := strings.Index(line, "refs/tags/")
		if i < 0 {
			continue
		}
		raw := strings.TrimSpace(strings.TrimSuffix(line[i+len("refs/tags/"):], "^{}"))

		v, err := version.NewVersion(strings.TrimPrefix(raw, "v"))
		if err != nil || v.Prerelease() != "" {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best, bestRaw = v, raw
		}
	}

	if best == nil {
		return "", errors.New("no stable release tag found")
	}

	return bestRaw, nil
}

// runCompatCheck fetches the integration's owner-maintained compatibility check
// at ref and runs it against projectDir. The script's report is written to out
// (stderr, so --format json stdout stays clean); a non-zero exit means the
// project is incompatible and the install must not proceed. It is a package var
// so tests can substitute it without network access.
var runCompatCheck = func(ctx context.Context, repo, skill, ref, projectDir string, out io.Writer) error {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/skills/%s/scripts/compatibility-check.sh", repo, ref, skill)

	script, err := httpGet(ctx, url)
	if err != nil {
		return fmt.Errorf("cannot fetch the compatibility check for %s@%s: %w", skill, ref, err)
	}

	// The script reads the project root as its first argument.
	cmd := exec.CommandContext(ctx, "bash", "-s", "--", projectDir)
	cmd.Stdin = bytes.NewReader(script)
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s@%s is not compatible with this project (see the report above): %w", skill, ref, err)
	}

	return nil
}

// maxCompatCheckBytes caps the compatibility-check download. The script is a
// small shell file; anything larger is treated as an error rather than fed to
// bash.
const maxCompatCheckBytes = 1 << 20 // 1 MiB

// httpGet fetches url and returns its body, erroring on any non-200 status. The
// request has a finite timeout and the body is bounded, so a stalled or
// oversized response cannot hang the command or exhaust memory.
func httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCompatCheckBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxCompatCheckBytes {
		return nil, fmt.Errorf("GET %s: response exceeds %d bytes", url, maxCompatCheckBytes)
	}

	return body, nil
}

package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/shyim/go-version"
)

// errNotFound marks an HTTP 404 for a friendlier caller message.
var errNotFound = errors.New("not found")

// skillsVersion pins the skills.sh CLI the commands run against.
const skillsVersion = "1.5.18"

// skillsAddArgs builds the `npx skills add ...` argv. The source is a tree URL
// (see skillSourceURL) pinning the skill, so no `--skill` is needed.
func skillsAddArgs(source, agent string, global, assumeYes bool) []string {
	argv := []string{
		"npx", "--yes", "skills@" + skillsVersion, "add", source,
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

// validateAgent rejects --agent shapes skills.sh cannot round-trip: empty, the
// "*" wildcard, and comma/whitespace lists. The name itself is not checked.
func validateAgent(agent string) error {
	if agent == "" {
		return errors.New("specify the target agent with --agent (e.g. --agent claude-code)")
	}
	if strings.ContainsAny(agent, "*, \t") {
		return fmt.Errorf("--agent takes a single agent (e.g. claude-code); %q is not supported — run the command once per agent", agent)
	}

	return nil
}

// skillSourceURL builds the GitHub tree URL pinning a skill to a ref, e.g.
// .../shopware-cli/tree/0.18.3/skills/shopware-cli. A bare owner/repo@ref is
// ignored by skills.sh (it installs the default branch).
func skillSourceURL(repoURL, ref, skill string) string {
	return fmt.Sprintf("https://github.com/%s/tree/%s/skills/%s", ownerRepo(repoURL), ref, skill)
}

// skillsRemoveArgs builds the `npx skills remove ...` argv.
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

// runSkills runs a skills.sh command via npx in dir (empty = current directory),
// streaming output to out. A package var so tests can replace it.
var runSkills = func(ctx context.Context, argv []string, dir string, out io.Writer) error {
	if _, err := exec.LookPath(argv[0]); err != nil {
		return fmt.Errorf("%s not found: installing skills requires Node.js/npx on PATH", argv[0])
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("skills %s failed: %w", skillsOp(argv), err)
	}

	return nil
}

// skillsOp returns the skills.sh subcommand from argv, for error messages.
func skillsOp(argv []string) string {
	if len(argv) > 3 {
		return argv[3]
	}

	return "command"
}

// ownerRepo turns a GitHub repo URL into "owner/repo" form.
func ownerRepo(repoURL string) string {
	s := strings.TrimSuffix(repoURL, ".git")
	s = strings.TrimPrefix(s, "https://github.com/")
	s = strings.TrimPrefix(s, "http://github.com/")

	return s
}

// resolveLatestTag returns the highest stable release tag of repoURL via
// `git ls-remote`. A package var so tests can replace it.
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

// verifyTag errors unless ref is an existing tag of repoURL (skills.sh would
// silently install the default branch otherwise). A package var for tests.
var verifyTag = func(ctx context.Context, repoURL, ref string) error {
	out, err := exec.CommandContext(ctx, "git", "ls-remote", "--tags", repoURL, "refs/tags/"+ref).Output()
	if err != nil {
		return fmt.Errorf("cannot verify tag %s of %s: %w", ref, repoURL, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("release %q not found in %s", ref, repoURL)
	}

	return nil
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

// compatReport is the JSON verdict the compatibility check prints.
type compatReport struct {
	Compatible bool     `json:"compatible"`
	Errors     []string `json:"errors"`
	Warnings   []string `json:"warnings"`
}

// runCompatCheck fetches the owner compatibility check at ref and runs it against
// projectDir. A package var so tests can replace it.
var runCompatCheck = func(ctx context.Context, repo, skill, ref, projectDir string, out io.Writer) error {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/skills/%s/scripts/compatibility-check.sh", repo, ref, skill)

	script, err := httpGet(ctx, url)
	if errors.Is(err, errNotFound) {
		return fmt.Errorf("release %q does not include the %s skill (it may predate it) — try a newer release", ref, skill)
	}
	if err != nil {
		return fmt.Errorf("cannot fetch the compatibility check for %s@%s: %w", skill, ref, err)
	}

	// The script reads the project root as its first argument and prints a JSON
	// verdict on stdout.
	cmd := exec.CommandContext(ctx, "bash", "-s", "--", projectDir)
	cmd.Stdin = bytes.NewReader(script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	return interpretCompatOutput(stdout.Bytes(), strings.TrimSpace(stderr.String()), skill, ref, runErr, out)
}

// interpretCompatOutput renders the verdict: parsed JSON means incompatible (show
// reasons), no JSON means the check could not run (e.g. PHP missing).
func interpretCompatOutput(stdout []byte, stderr, skill, ref string, runErr error, out io.Writer) error {
	var report compatReport
	if json.Unmarshal(bytes.TrimSpace(stdout), &report) != nil {
		if stderr != "" {
			return fmt.Errorf("could not run the compatibility check for %s@%s: %s", skill, ref, stderr)
		}

		return fmt.Errorf("could not run the compatibility check for %s@%s (is PHP available?): %w", skill, ref, runErr)
	}

	if report.Compatible {
		for _, w := range report.Warnings {
			_, _ = fmt.Fprintf(out, "warning: %s\n", w)
		}

		return nil
	}

	_, _ = fmt.Fprintf(out, "%s@%s is not compatible with this project:\n", skill, ref)
	for _, e := range report.Errors {
		_, _ = fmt.Fprintf(out, "  - %s\n", e)
	}
	for _, w := range report.Warnings {
		_, _ = fmt.Fprintf(out, "  (warning) %s\n", w)
	}

	return fmt.Errorf("%s@%s is not compatible with this project", skill, ref)
}

// maxCompatCheckBytes caps the compatibility-check download (a small shell file).
const maxCompatCheckBytes = 1 << 20 // 1 MiB

// httpGet fetches url with a timeout and a bounded body, erroring on non-200.
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

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("GET %s: %w", url, errNotFound)
	}
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

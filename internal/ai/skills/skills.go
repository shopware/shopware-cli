// Package skills is a thin client over the skills.sh CLI (via npx) plus the git
// and HTTP lookups the ai commands need: building argv, resolving release tags,
// and running an integration's compatibility check.
package skills

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

// skillsVersion pins the skills.sh major line; npx resolves the latest 1.x, so we
// get fixes but never a breaking 2.0.
const skillsVersion = "1"

// AddArgs builds the `npx skills add ...` argv. The source is a tree URL (see
// SourceURL) pinning the skill, so no `--skill` is needed.
func AddArgs(source, agent string, global, assumeYes bool) []string {
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

// SourceURL builds the GitHub tree URL pinning a skill to a ref, e.g.
// .../shopware-cli/tree/0.18.3/skills/shopware-cli. A bare owner/repo@ref is
// ignored by skills.sh (it installs the default branch).
func SourceURL(repoURL, ref, skill string) string {
	return fmt.Sprintf("https://github.com/%s/tree/%s/skills/%s", OwnerRepo(repoURL), ref, skill)
}

// OwnerRepo turns a GitHub repo URL into "owner/repo" form.
func OwnerRepo(repoURL string) string {
	s := strings.TrimSuffix(repoURL, ".git")
	s = strings.TrimPrefix(s, "https://github.com/")
	s = strings.TrimPrefix(s, "http://github.com/")

	return s
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

// remoteTagNames lists the tag names of repoURL via `git ls-remote --tags`.
func remoteTagNames(ctx context.Context, repoURL string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "git", "ls-remote", "--tags", repoURL).Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list release tags of %s: %w", repoURL, err)
	}

	return tagNames(strings.Split(string(out), "\n")), nil
}

// tagNames extracts tag names from `git ls-remote --tags` lines
// ("<sha>\trefs/tags/<tag>"), dropping the peeled "^{}" suffix.
func tagNames(lsRemoteLines []string) []string {
	var tags []string
	for _, line := range lsRemoteLines {
		i := strings.Index(line, "refs/tags/")
		if i < 0 {
			continue
		}
		tags = append(tags, strings.TrimSpace(strings.TrimSuffix(line[i+len("refs/tags/"):], "^{}")))
	}

	return tags
}

// matchTag returns the tag matching want, ignoring a leading "v" on either side
// (repos tag releases inconsistently, e.g. "0.1.10" but "v0.1.9").
func matchTag(tags []string, want string) (string, bool) {
	norm := strings.TrimPrefix(want, "v")
	for _, tag := range tags {
		if strings.TrimPrefix(tag, "v") == norm {
			return tag, true
		}
	}

	return "", false
}

// ResolveLatestTag returns the highest stable release tag of repoURL.
func ResolveLatestTag(ctx context.Context, repoURL string) (string, error) {
	tags, err := remoteTagNames(ctx, repoURL)
	if err != nil {
		return "", err
	}

	tag, err := latestStableTag(tags)
	if err != nil {
		return "", fmt.Errorf("%s: %w", repoURL, err)
	}

	return tag, nil
}

// ResolveTag returns the actual tag of repoURL matching want, accepting a "v"
// prefix on either side. skills.sh would silently install the default branch for
// a missing ref, so the tag is resolved up front.
func ResolveTag(ctx context.Context, repoURL, want string) (string, error) {
	tags, err := remoteTagNames(ctx, repoURL)
	if err != nil {
		return "", err
	}
	if tag, ok := matchTag(tags, want); ok {
		return tag, nil
	}

	return "", fmt.Errorf("cannot find release %q in %s", want, repoURL)
}

// latestStableTag picks the highest stable semver tag; pre-releases are ignored.
func latestStableTag(tags []string) (string, error) {
	var best *version.Version
	var bestRaw string

	for _, raw := range tags {
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

// RunCompatCheck fetches the owner compatibility check at ref and runs it against
// projectDir.
func RunCompatCheck(ctx context.Context, repo, skill, ref, projectDir string, out io.Writer) error {
	// The check runs locally and needs PHP; without it the script reports the
	// project as incompatible instead of signalling a missing tool, so check up
	// front (a Docker project may not expose PHP on the host).
	if _, err := exec.LookPath("php"); err != nil {
		return fmt.Errorf("cannot run the compatibility check for %s: PHP is not on PATH", skill)
	}

	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/skills/%s/scripts/compatibility-check.sh", repo, ref, skill)

	script, err := httpGet(ctx, url)
	if errors.Is(err, errNotFound) {
		return fmt.Errorf("release %q does not include the %s skill (it may predate it), try a newer release", ref, skill)
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
			return fmt.Errorf("cannot run the compatibility check for %s@%s: %s", skill, ref, stderr)
		}

		return fmt.Errorf("cannot run the compatibility check for %s@%s (is PHP available?): %w", skill, ref, runErr)
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

package verifier

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shyim/go-version"
)

// ResolveTargetVersion turns a --target-version input into a concrete Shopware release.
// A tag matches as typed; a shorter version picks the newest stable release with that prefix.
func ResolveTargetVersion(requested string, known []string) (string, error) {
	input := normalizeRequested(requested)
	if input == "" {
		return "", errors.New("--target-version must not be empty")
	}

	all := parseVersions(known)
	stable := stableReleases(all)

	for _, v := range all {
		if strings.EqualFold(v.String(), input) {
			return v.String(), nil
		}
	}

	segments := strings.Split(input, ".")
	wanted, err := version.NewVersion(input)
	if err != nil || !isPlainVersion(input) || len(segments) < 2 || len(segments) > 4 {
		return "", fmt.Errorf("--target-version %q is not a version. Pass a release such as %s, or a minor such as %s to use its newest release", requested, exampleRelease(stable), exampleMinor(stable))
	}

	if len(segments) < 4 {
		for i := len(stable) - 1; i >= 0; i-- {
			if hasSegmentPrefix(stable[i], segments) {
				return stable[i].String(), nil
			}
		}
	}

	return "", unknownReleaseError(requested, wanted, segments, stable)
}

// TargetVersionCompletions lists minors and stable releases, newest first, for shell completion.
func TargetVersionCompletions(ctx context.Context) []string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	known, err := getShopwareVersions(ctx)
	if err != nil {
		return nil
	}

	return targetVersionCompletions(known)
}

func targetVersionCompletions(known []string) []string {
	stable := stableReleases(parseVersions(known))
	seenMinor := map[string]bool{}
	completions := make([]string, 0, len(stable))

	for i := len(stable) - 1; i >= 0; i-- {
		minor := minorOf(stable[i])
		if !seenMinor[minor] {
			seenMinor[minor] = true
			completions = append(completions, minor)
		}

		completions = append(completions, stable[i].String())
	}

	return completions
}

// unknownReleaseError points at the nearest releases so the user can correct the input.
func unknownReleaseError(requested string, wanted *version.Version, segments []string, stable []*version.Version) error {
	if len(stable) == 0 {
		return fmt.Errorf("--target-version %q is not a Shopware release", requested)
	}

	siblings := make([]string, 0, 3)
	for i := len(stable) - 1; i >= 0 && len(siblings) < 3; i-- {
		if hasSegmentPrefix(stable[i], segments[:2]) {
			siblings = append(siblings, stable[i].String())
		}
	}

	if len(siblings) > 0 {
		return fmt.Errorf("--target-version %q is not a Shopware release. Closest %s releases: %s", requested, segments[0]+"."+segments[1], strings.Join(siblings, ", "))
	}

	newest := stable[len(stable)-1]
	if wanted.GreaterThan(newest) {
		return fmt.Errorf("--target-version %q has no Shopware release yet. Newest release: %s", requested, newest)
	}

	return fmt.Errorf("--target-version %q is not a Shopware release. Newest release: %s", requested, newest)
}

// parseVersions drops unparsable tags and sorts ascending.
func parseVersions(known []string) []*version.Version {
	parsed := make([]*version.Version, 0, len(known))

	for _, raw := range known {
		v, err := version.NewVersion(raw)
		if err != nil {
			continue
		}

		parsed = append(parsed, v)
	}

	sort.Sort(version.Collection(parsed))

	return parsed
}

func stableReleases(all []*version.Version) []*version.Version {
	stable := make([]*version.Version, 0, len(all))

	for _, v := range all {
		if !v.IsPrerelease() {
			stable = append(stable, v)
		}
	}

	return stable
}

func normalizeRequested(requested string) string {
	return strings.TrimPrefix(strings.TrimSpace(requested), "v")
}

// isExactRelease reports whether the input names one release without needing the release list.
func isExactRelease(input string) bool {
	return isPlainVersion(input) && len(strings.Split(input, ".")) == 4
}

func isPlainVersion(input string) bool {
	for _, r := range input {
		if r != '.' && (r < '0' || r > '9') {
			return false
		}
	}

	return input != ""
}

func hasSegmentPrefix(v *version.Version, segments []string) bool {
	actual := v.Segments()

	for i, segment := range segments {
		n, err := strconv.Atoi(segment)
		if err != nil || i >= len(actual) || actual[i] != n {
			return false
		}
	}

	return true
}

func minorOf(v *version.Version) string {
	segments := v.Segments()

	return fmt.Sprintf("%d.%d", segments[0], segments[1])
}

func exampleRelease(stable []*version.Version) string {
	if len(stable) == 0 {
		return "6.7.0.0"
	}

	return stable[len(stable)-1].String()
}

func exampleMinor(stable []*version.Version) string {
	if len(stable) == 0 {
		return "6.7"
	}

	return minorOf(stable[len(stable)-1])
}

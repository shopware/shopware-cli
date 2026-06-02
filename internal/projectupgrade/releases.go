package projectupgrade

import (
	"sort"

	"github.com/shyim/go-version"
)

// branch identifies a Shopware release branch by its major and minor segment
// (e.g. 6.6). Shopware ships feature releases on the minor segment, so the
// "next" branch is the current one with its minor incremented.
type branch struct {
	major int
	minor int
}

func branchOf(v *version.Version) branch {
	return branch{major: v.Major(), minor: v.Minor()}
}

func (b branch) next() branch {
	return branch{major: b.major, minor: b.minor + 1}
}

// FilterUpdateVersions returns the upgrade target versions appropriate for
// currentVersion: the next branch's releases first (newest first), followed
// by the remaining patches of the current branch. This mirrors the version
// filtering applied by `ReleaseInfoProvider::fetchUpdateVersions` in
// shopware/web-installer.
//
// Pre-releases (RC/beta/alpha) and any version older than or equal to
// currentVersion are dropped.
func FilterUpdateVersions(currentVersion *version.Version, allVersions []string) []string {
	parsed := make([]*version.Version, 0, len(allVersions))

	for _, raw := range allVersions {
		v, err := version.NewVersion(raw)
		if err != nil {
			continue
		}

		if v.IsPrerelease() {
			continue
		}

		if !v.GreaterThan(currentVersion) {
			continue
		}

		parsed = append(parsed, v)
	}

	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i].GreaterThan(parsed[j])
	})

	byBranch := map[branch][]string{}
	for _, v := range parsed {
		b := branchOf(v)
		byBranch[b] = append(byBranch[b], v.String())
	}

	currentBranch := branchOf(currentVersion)

	result := make([]string, 0)
	result = append(result, byBranch[currentBranch.next()]...)
	result = append(result, byBranch[currentBranch]...)

	return result
}

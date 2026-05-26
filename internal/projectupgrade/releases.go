package projectupgrade

import (
	"sort"
	"strconv"
	"strings"

	"github.com/shyim/go-version"
)

// FilterUpdateVersions returns the upgrade target versions appropriate for
// currentVersion: the next major version's releases first (newest first),
// followed by the remaining patches of the current major. This mirrors the
// version filtering applied by `ReleaseInfoProvider::fetchUpdateVersions` in
// shopware/web-installer.
//
// Release candidates and any version older than or equal to currentVersion
// are dropped.
func FilterUpdateVersions(currentVersion *version.Version, allVersions []string) []string {
	parsed := make([]*version.Version, 0, len(allVersions))

	for _, raw := range allVersions {
		if strings.Contains(strings.ToLower(raw), "rc") {
			continue
		}

		v, err := version.NewVersion(raw)
		if err != nil {
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

	byMajor := map[string][]string{}
	for _, v := range parsed {
		major := majorBranch(v)
		byMajor[major] = append(byMajor[major], v.String())
	}

	currentMajor := majorBranch(currentVersion)
	nextMajor := nextMajor(currentMajor)

	result := make([]string, 0)
	if list, ok := byMajor[nextMajor]; ok {
		result = append(result, list...)
	}
	if list, ok := byMajor[currentMajor]; ok {
		result = append(result, list...)
	}

	return result
}

func majorBranch(v *version.Version) string {
	segments := v.Segments()
	if len(segments) < 2 {
		return v.String()
	}

	return strconv.Itoa(segments[0]) + "." + strconv.Itoa(segments[1])
}

func nextMajor(currentMajor string) string {
	parts := strings.SplitN(currentMajor, ".", 2)
	if len(parts) != 2 {
		return currentMajor
	}

	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return currentMajor
	}

	return parts[0] + "." + strconv.Itoa(minor+1)
}

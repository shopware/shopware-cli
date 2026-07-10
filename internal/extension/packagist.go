package extension

import (
	"context"
	"fmt"
	"sort"

	"github.com/shyim/go-composer/repository"
	"github.com/shyim/go-version"
)

func GetShopwareVersions(ctx context.Context) ([]string, error) {
	pkg, err := repository.New(repository.PackagistURL, nil).GetPackage(ctx, "shopware/core")
	if err != nil {
		return nil, fmt.Errorf("get package versions: %w", err)
	}

	versions := make([]string, 0, len(pkg.Versions))

	for _, packageVersion := range pkg.Versions {
		versions = append(versions, packageVersion.VersionNormalized)
	}

	return versions, nil
}

func lookupForMinMatchingVersion(ctx context.Context, versionConstraint *version.Constraints) (string, error) {
	versions, err := GetShopwareVersions(ctx)
	if err != nil {
		return "", fmt.Errorf("get shopware versions: %w", err)
	}

	return getMinMatchingVersion(versionConstraint, versions), nil
}

const DevVersionNumber = "6.9.9.9"

func getMinMatchingVersion(constraint *version.Constraints, versions []string) string {
	vs := make([]*version.Version, 0)

	for _, r := range versions {
		v, err := version.NewVersion(r)
		if err != nil {
			continue
		}

		vs = append(vs, v)
	}

	sort.Sort(version.Collection(vs))

	matchingVersions := make([]*version.Version, 0)

	for _, v := range vs {
		if constraint.Check(v) {
			matchingVersions = append(matchingVersions, v)
		}
	}

	// If there are matching versions, return the first non-prerelease version
	for _, matchingVersion := range matchingVersions {
		if matchingVersion.IsPrerelease() {
			continue
		}

		return matchingVersion.String()
	}

	// If there are no non-prerelease versions, return the first matching version
	if len(matchingVersions) > 0 {
		return matchingVersions[0].String()
	}

	return DevVersionNumber
}

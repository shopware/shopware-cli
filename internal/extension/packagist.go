package extension

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/logging"
)

type packagistResponse struct {
	Packages struct {
		Core []struct {
			Version string `json:"version_normalized"`
		} `json:"shopware/core"`
	} `json:"packages"`
}

func GetShopwareVersions(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://repo.packagist.org/p2/shopware/core.json", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create composer version request: %w", err)
	}

	req.Header.Set("User-Agent", "Shopware CLI")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch composer versions: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("lookupForMinMatchingVersion: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch composer versions: %s", resp.Status)
	}

	var pckResponse packagistResponse

	var versions []string

	if err := json.NewDecoder(resp.Body).Decode(&pckResponse); err != nil {
		return nil, fmt.Errorf("decode composer versions: %w", err)
	}

	for _, v := range pckResponse.Packages.Core {
		versions = append(versions, v.Version)
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

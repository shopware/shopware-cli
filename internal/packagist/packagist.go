package packagist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/shopware/shopware-cli/logging"
)

type PackageResponse struct {
	Packages map[string]map[string]PackageVersion `json:"packages"`
}

func (p *PackageResponse) HasPackage(name string) bool {
	expectedName := fmt.Sprintf("store.shopware.com/%s", strings.ToLower(name))

	_, ok := p.Packages[expectedName]

	return ok
}

type PackageVersion struct {
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Replace     map[string]string `json:"replace"`
}

type ComposerPackageVersion struct {
	Name              string            `json:"name"`
	Version           string            `json:"version"`
	VersionNormalized string            `json:"version_normalized"`
	Description       string            `json:"description"`
	Time              string            `json:"time"`
	Replace           map[string]string `json:"replace"`
}

type composerPackageVersionsResponse struct {
	Minified string                                  `json:"minified"`
	Packages map[string][]map[string]json.RawMessage `json:"packages"`
}

func GetAvailablePackagesFromShopwareStore(ctx context.Context, token string) (*PackageResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://packages.shopware.com/packages.json", nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Shopware CLI")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("Cannot close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get packages: %s", resp.Status)
	}

	var packages PackageResponse
	if err := json.NewDecoder(resp.Body).Decode(&packages); err != nil {
		return nil, err
	}

	return &packages, nil
}

func GetShopwarePackageVersions(ctx context.Context) ([]ComposerPackageVersion, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://repo.packagist.org/p2/shopware/core.json", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create package versions request: %w", err)
	}

	req.Header.Set("User-Agent", "Shopware CLI")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch package versions: %w", err)
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("Cannot close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch package versions: %s", resp.Status)
	}

	var packageResponse composerPackageVersionsResponse

	if err := json.NewDecoder(resp.Body).Decode(&packageResponse); err != nil {
		return nil, fmt.Errorf("decode package versions: %w", err)
	}

	rawVersions, ok := packageResponse.Packages["shopware/core"]
	if !ok {
		return nil, fmt.Errorf("decode package versions: package shopware/core not found")
	}

	if packageResponse.Minified != "" {
		rawVersions = unminifyComposerMetadata(rawVersions)
	}

	versions := make([]ComposerPackageVersion, 0, len(rawVersions))

	for _, rawVersion := range rawVersions {
		payload, err := json.Marshal(rawVersion)
		if err != nil {
			return nil, fmt.Errorf("decode package versions: %w", err)
		}

		var version ComposerPackageVersion

		if err := json.Unmarshal(payload, &version); err != nil {
			return nil, fmt.Errorf("decode package versions: %w", err)
		}

		versions = append(versions, version)
	}

	return versions, nil
}

func unminifyComposerMetadata(versions []map[string]json.RawMessage) []map[string]json.RawMessage {
	if len(versions) == 0 {
		return nil
	}

	expanded := make([]map[string]json.RawMessage, 0, len(versions))
	var expandedVersion map[string]json.RawMessage

	for _, versionData := range versions {
		if expandedVersion == nil {
			expandedVersion = cloneRawMessageMap(versionData)
			expanded = append(expanded, cloneRawMessageMap(expandedVersion))

			continue
		}

		for key, val := range versionData {
			if bytes.Equal(val, []byte(`"__unset"`)) {
				delete(expandedVersion, key)
			} else {
				expandedVersion[key] = val
			}
		}

		expanded = append(expanded, cloneRawMessageMap(expandedVersion))
	}

	return expanded
}

func cloneRawMessageMap(in map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(in))

	for key, val := range in {
		out[key] = val
	}

	return out
}

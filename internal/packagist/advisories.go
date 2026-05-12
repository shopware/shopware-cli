package packagist

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/logging"
)

// SecurityAdvisory describes a single packagist security advisory for a package.
type SecurityAdvisory struct {
	AdvisoryID       string `json:"advisoryId"`
	PackageName      string `json:"packageName"`
	RemoteID         string `json:"remoteId"`
	Title            string `json:"title"`
	Link             string `json:"link"`
	CVE              string `json:"cve"`
	AffectedVersions string `json:"affectedVersions"`
	Source           string `json:"source"`
	ReportedAt       string `json:"reportedAt"`
	Severity         string `json:"severity"`
}

// Affects reports whether the given version is covered by this advisory's
// affectedVersions constraint. The packagist API uses `|` to separate OR
// branches; each branch is an AND of comma-separated constraints.
func (a SecurityAdvisory) Affects(targetVersion string) bool {
	v, err := version.NewVersion(strings.TrimPrefix(targetVersion, "v"))
	if err != nil {
		return false
	}

	for _, branch := range strings.Split(a.AffectedVersions, "|") {
		branch = strings.TrimSpace(branch)
		if branch == "" {
			continue
		}
		cs, err := version.NewConstraint(branch)
		if err != nil {
			continue
		}
		if cs.Check(v) {
			return true
		}
	}
	return false
}

type advisoriesResponse struct {
	Advisories map[string][]SecurityAdvisory `json:"advisories"`
}

// GetShopwareSecurityAdvisories fetches all known security advisories for
// shopware/core from packagist.
func GetShopwareSecurityAdvisories(ctx context.Context) ([]SecurityAdvisory, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://packagist.org/api/security-advisories/?packages%5B%5D=shopware/core", http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("create advisories request: %w", err)
	}
	req.Header.Set("User-Agent", "Shopware CLI")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch advisories: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("Cannot close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch advisories: %s", resp.Status)
	}

	var parsed advisoriesResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode advisories: %w", err)
	}

	return parsed.Advisories["shopware/core"], nil
}

// FilterAdvisoriesForVersion returns only the advisories that affect the given version.
func FilterAdvisoriesForVersion(advisories []SecurityAdvisory, chosenVersion string) []SecurityAdvisory {
	var matching []SecurityAdvisory
	for _, a := range advisories {
		if a.Affects(chosenVersion) {
			matching = append(matching, a)
		}
	}
	return matching
}

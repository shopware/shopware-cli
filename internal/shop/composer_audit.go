package shop

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// ComposerAdvisory is one security advisory reported by `composer audit`.
type ComposerAdvisory struct {
	AdvisoryID  string `json:"advisoryId"`
	PackageName string `json:"packageName"`
	Title       string `json:"title"`
	CVE         string `json:"cve"`
	Link        string `json:"link"`
}

// ComposerAudit holds the security advisories reported by `composer audit`,
// keyed by package name. Other findings such as abandoned packages are not
// included.
type ComposerAudit struct {
	Advisories map[string][]ComposerAdvisory
}

// Count returns the number of security advisories across all packages.
func (a ComposerAudit) Count() int {
	n := 0
	for _, advisories := range a.Advisories {
		n += len(advisories)
	}
	return n
}

// Packages returns the names of the affected packages, sorted.
func (a ComposerAudit) Packages() []string {
	return slices.Sorted(maps.Keys(a.Advisories))
}

// ParseComposerAudit parses the output of `composer audit --format=json`.
// Composer encodes an empty advisory list as `[]` instead of an object.
func ParseComposerAudit(output []byte) (ComposerAudit, error) {
	var raw struct {
		Advisories json.RawMessage `json:"advisories"`
	}
	if err := json.Unmarshal(output, &raw); err != nil {
		return ComposerAudit{}, fmt.Errorf("cannot parse composer audit output: %w", err)
	}

	audit := ComposerAudit{Advisories: map[string][]ComposerAdvisory{}}
	if len(raw.Advisories) == 0 || strings.TrimSpace(string(raw.Advisories)) == "[]" {
		return audit, nil
	}

	var packages map[string]json.RawMessage
	if err := json.Unmarshal(raw.Advisories, &packages); err != nil {
		return ComposerAudit{}, fmt.Errorf("cannot parse composer audit advisories: %w", err)
	}

	for name, data := range packages {
		advisories, err := parseAdvisoryList(data)
		if err != nil {
			return ComposerAudit{}, fmt.Errorf("cannot parse composer audit advisories for %s: %w", name, err)
		}
		audit.Advisories[name] = advisories
	}

	return audit, nil
}

// parseAdvisoryList decodes a package's advisories. Composer writes them as a
// list, or as an object keyed by index when it filtered some of them out.
func parseAdvisoryList(data json.RawMessage) ([]ComposerAdvisory, error) {
	var list []ComposerAdvisory
	if err := json.Unmarshal(data, &list); err == nil {
		return list, nil
	}

	var byIndex map[int]ComposerAdvisory
	if err := json.Unmarshal(data, &byIndex); err != nil {
		return nil, err
	}

	for _, key := range slices.Sorted(maps.Keys(byIndex)) {
		list = append(list, byIndex[key])
	}
	return list, nil
}

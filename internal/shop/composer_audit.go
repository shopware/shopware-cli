package shop

import (
	"encoding/json"
	"fmt"
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
	names := make([]string, 0, len(a.Advisories))
	for name := range a.Advisories {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
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

	if err := json.Unmarshal(raw.Advisories, &audit.Advisories); err != nil {
		return ComposerAudit{}, fmt.Errorf("cannot parse composer audit advisories: %w", err)
	}

	return audit, nil
}

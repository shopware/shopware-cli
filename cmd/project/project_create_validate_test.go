package project

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/shyim/go-composer/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAdvisoryProvider serves canned security advisories through go-composer's
// repository handler; the Package capability is never used by the gate.
type fakeAdvisoryProvider struct {
	advisories map[string][]repository.SecurityAdvisory
}

func (fakeAdvisoryProvider) Package(context.Context, string) (*repository.Package, error) {
	return nil, repository.ErrPackageNotFound
}

func (p fakeAdvisoryProvider) SecurityAdvisories(context.Context, []string) (map[string][]repository.SecurityAdvisory, error) {
	return p.advisories, nil
}

func TestCheckSecurityAdvisoriesNonInteractiveGate(t *testing.T) {
	srv := httptest.NewServer(repository.NewHandler(fakeAdvisoryProvider{advisories: map[string][]repository.SecurityAdvisory{
		"shopware/core": {{
			PackageName:      "shopware/core",
			Title:            "Test advisory",
			CVE:              "CVE-2026-0001",
			Severity:         "high",
			AffectedVersions: "<6.6.10.0",
		}},
	}}))
	t.Cleanup(srv.Close)

	oldURL := packagistURL
	packagistURL = srv.URL
	t.Cleanup(func() { packagistURL = oldURL })

	t.Run("affected version blocks without no-audit", func(t *testing.T) {
		opts := &createOptions{}
		err := checkSecurityAdvisories(t.Context(), opts, "6.6.5.0")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--no-audit")
	})

	t.Run("no-audit proceeds", func(t *testing.T) {
		opts := &createOptions{noAudit: true}
		require.NoError(t, checkSecurityAdvisories(t.Context(), opts, "6.6.5.0"))
	})

	t.Run("unaffected version passes", func(t *testing.T) {
		opts := &createOptions{}
		require.NoError(t, checkSecurityAdvisories(t.Context(), opts, "6.7.0.0"))
	})
}

func TestRenderSecurityAdvisoriesFormatsSeverityAndCVE(t *testing.T) {
	advisories := []repository.SecurityAdvisory{
		{Title: "First issue", CVE: "CVE-2026-0001", Link: "https://example.com/adv", Severity: "high"},
		{Title: "Second issue"},
	}

	out := renderSecurityAdvisories("6.6.5.0", advisories)
	assert.Contains(t, out, "Security Advisories for Shopware 6.6.5.0")
	assert.Contains(t, out, "HIGH")
	assert.Contains(t, out, "UNKNOWN")
	assert.Contains(t, out, "CVE-2026-0001")
	assert.Contains(t, out, "https://example.com/adv")

	assert.Equal(t, "advisory", pluralize(1, "advisory", "advisories"))
	assert.Equal(t, "advisories", pluralize(2, "advisory", "advisories"))
}

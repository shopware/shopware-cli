package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseComposerAuditCountsAdvisoriesOnly(t *testing.T) {
	audit, err := ParseComposerAudit([]byte(`{
		"advisories": {
			"symfony/http-kernel": [
				{"advisoryId": "a", "packageName": "symfony/http-kernel", "title": "First", "cve": "CVE-1", "link": "https://example.com/a"},
				{"advisoryId": "b", "packageName": "symfony/http-kernel", "title": "Second", "cve": null, "link": null}
			],
			"guzzlehttp/psr7": [
				{"advisoryId": "c", "packageName": "guzzlehttp/psr7", "title": "Third"}
			]
		},
		"abandoned": {"swiftmailer/swiftmailer": "symfony/mailer"}
	}`))

	require.NoError(t, err)
	assert.Equal(t, 3, audit.Count())
	assert.Equal(t, []string{"guzzlehttp/psr7", "symfony/http-kernel"}, audit.Packages())
	assert.Equal(t, "https://example.com/a", audit.Advisories["symfony/http-kernel"][0].Link)
}

func TestParseComposerAuditWithoutAdvisories(t *testing.T) {
	audit, err := ParseComposerAudit([]byte(`{"advisories": [], "abandoned": {"foo/bar": null}}`))

	require.NoError(t, err)
	assert.Equal(t, 0, audit.Count())
}

func TestParseComposerAuditInvalidOutput(t *testing.T) {
	_, err := ParseComposerAudit([]byte("No packages - skipping audit."))

	assert.Error(t, err)
}

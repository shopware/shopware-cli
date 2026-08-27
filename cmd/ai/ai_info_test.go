package ai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInfoJSON(t *testing.T) {
	out, err := executeAI(t, newAIInfoCmd(), "deployment-helper", "--json")
	require.NoError(t, err)

	const want = `{
	  "name": "deployment-helper",
	  "displayName": "Shopware Deployment Helper",
	  "type": "skill",
	  "provider": "shopware",
	  "description": "Use Shopware CLI and Deployment Helper together for build and deploy workflows.",
	  "status": "coming-soon",
	  "available": false,
	  "availabilityReason": "not yet released",
	  "documentation": "https://developer.shopware.com/docs/guides/hosting/installation-updates/deployments/deployment-helper/index.html",
	  "delivery": {"kind": "git", "repository": "https://github.com/shopware/deployment-helper"},
	  "compatibility": {"source": "owner"}
	}`

	assert.JSONEq(t, want, out)

	// maintainer metadata never appears in output.
	assert.NotContains(t, strings.ToLower(out), "maintainer")
}

func TestInfoHumanContains(t *testing.T) {
	out, err := executeAI(t, newAIInfoCmd(), "shopware-cli")
	require.NoError(t, err)

	for _, want := range []string{
		"shopware-cli",
		"Documentation:",
		"https://developer.shopware.com/docs/products/cli/",
		"bundled",
		"yes",
	} {
		assert.Contains(t, out, want)
	}
}

func TestInfoUnknownNameErrors(t *testing.T) {
	_, err := executeAI(t, newAIInfoCmd(), "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown integration "does-not-exist"`)
}

package ai

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/directory"
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
		// AC: info shows compatibility requirements for every entry, including
		// bundled ones that have no explicit compatibility block.
		"Compatibility:",
		"none (bundled, always compatible)",
	} {
		assert.Contains(t, out, want)
	}
}

// TestInfoHumanGitEntry exercises the human output for a git-delivered entry:
// the "git (repository)" delivery rendering and the compatibility source line.
func TestInfoHumanGitEntry(t *testing.T) {
	out, err := executeAI(t, newAIInfoCmd(), "deployment-helper")
	require.NoError(t, err)

	for _, want := range []string{
		"Delivery:",
		"git (https://github.com/shopware/deployment-helper)",
		"Compatibility:",
		"owner",
		"no (not yet released)",
	} {
		assert.Contains(t, out, want)
	}
}

func TestCompatibilityLabel(t *testing.T) {
	// explicit compatibility block → its source
	withCompat := directory.Integration{Compatibility: &directory.Compatibility{Source: "owner"}}
	assert.Equal(t, "owner", compatibilityLabel(withCompat))

	// bundled entry with no block → always compatible
	bundled := directory.Integration{Delivery: directory.Delivery{Kind: directory.DeliveryBundled}}
	assert.Equal(t, "none (bundled, always compatible)", compatibilityLabel(bundled))

	// non-bundled entry with no block → plain "none"
	git := directory.Integration{Delivery: directory.Delivery{Kind: directory.DeliveryGit}}
	assert.Equal(t, "none", compatibilityLabel(git))
}

func TestInfoUnknownNameErrors(t *testing.T) {
	_, err := executeAI(t, newAIInfoCmd(), "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown integration "does-not-exist"`)
}

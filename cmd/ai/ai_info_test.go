package ai

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/directory"
)

func TestWriteInfoTable(t *testing.T) {
	e := directory.Integration{
		Name: "deployment-helper", DisplayName: "Shopware Deployment Helper",
		Type: directory.TypeSkill, Provider: "shopware",
		Status:      directory.StatusActive,
		Description: "desc", Documentation: "https://example.test/dh",
		Delivery:      directory.Delivery{Kind: directory.DeliveryGit, Repository: "https://github.com/shopware/deployment-helper"},
		Compatibility: &directory.Compatibility{Source: "owner"},
		Internal:      &directory.Internal{Maintainer: "@shopware/team"},
	}

	var buf bytes.Buffer
	require.NoError(t, writeInfoTable(&buf, e))
	out := buf.String()

	for _, want := range []string{
		"deployment-helper",
		"Documentation:",
		"git (https://github.com/shopware/deployment-helper)",
		"Compatibility:",
		"owner",
	} {
		assert.Contains(t, out, want)
	}

	// maintainer metadata is never rendered
	assert.NotContains(t, strings.ToLower(out), "maintainer")
}

func TestWriteInfoJSONExcludesInternal(t *testing.T) {
	e := directory.Integration{Name: "x", Internal: &directory.Internal{Maintainer: "@shopware/team"}}

	var buf bytes.Buffer
	require.NoError(t, writeInfoJSON(&buf, e))

	out := strings.ToLower(buf.String())
	assert.NotContains(t, out, "maintainer")
	assert.NotContains(t, out, "internal")
}

func TestCompatibilityLabel(t *testing.T) {
	withCompat := directory.Integration{Compatibility: &directory.Compatibility{Source: "owner"}}
	assert.Equal(t, "owner", compatibilityLabel(withCompat))

	bundled := directory.Integration{Delivery: directory.Delivery{Kind: directory.DeliveryBundled}}
	assert.Equal(t, "none (bundled, always compatible)", compatibilityLabel(bundled))

	git := directory.Integration{Delivery: directory.Delivery{Kind: directory.DeliveryGit}}
	assert.Equal(t, "none", compatibilityLabel(git))
}

func TestDeliveryLabel(t *testing.T) {
	assert.Equal(t, "git (https://example.test/r)", deliveryLabel(directory.Delivery{Kind: directory.DeliveryGit, Repository: "https://example.test/r"}))
	assert.Equal(t, "bundled", deliveryLabel(directory.Delivery{Kind: directory.DeliveryBundled}))
}

package ai

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/directory"
)

func TestWriteInfoTable(t *testing.T) {
	e := directory.Integration{
		Name:          "deployment-helper",
		DisplayName:   "Shopware Deployment Helper",
		Type:          directory.TypeSkill,
		Provider:      "shopware",
		Status:        directory.StatusActive,
		Description:   "desc",
		Documentation: "https://example.test/dh",
		Delivery:      directory.Delivery{Kind: directory.DeliveryGit, Repository: "shopware/deployment-helper"},
	}

	var buf bytes.Buffer
	require.NoError(t, writeInfoTable(&buf, e))

	assert.Contains(t, buf.String(), "deployment-helper")
	assert.Contains(t, buf.String(), "git (shopware/deployment-helper)")
}

func TestWriteInfoJSON(t *testing.T) {
	e := directory.Integration{
		Name: "deployment-helper", DisplayName: "DH", Type: directory.TypeSkill,
		Provider: "shopware", Status: directory.StatusActive, Description: "d",
		Documentation: "https://example.test/dh",
		Delivery:      directory.Delivery{Kind: directory.DeliveryGit, Repository: "shopware/deployment-helper"},
	}

	var buf bytes.Buffer
	require.NoError(t, writeInfoJSON(&buf, e))

	assert.Contains(t, buf.String(), `"name":"deployment-helper"`)
	assert.NotContains(t, buf.String(), "installed")
}

func TestDeliveryLabel(t *testing.T) {
	assert.Equal(t, "git (shopware/deployment-helper)", deliveryLabel(directory.Delivery{Kind: directory.DeliveryGit, Repository: "shopware/deployment-helper"}))
	assert.Equal(t, "git", deliveryLabel(directory.Delivery{Kind: directory.DeliveryGit}))
}

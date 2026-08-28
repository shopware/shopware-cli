package ai

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/ai/directory"
)

func TestWriteListJSON(t *testing.T) {
	entries := []directory.Integration{
		{
			Name: "x", DisplayName: "X", Type: directory.TypeSkill,
			Provider: "shopware", Description: "d", Status: directory.StatusActive,
			Available: true,
			// fields below must not appear in the list shape
			Documentation: "https://example.test/x",
			Delivery:      directory.Delivery{Kind: directory.DeliveryBundled},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, writeListJSON(&buf, entries))

	assert.JSONEq(t, `[{"name":"x","displayName":"X","type":"skill","provider":"shopware","description":"d","status":"active","available":true}]`, buf.String())
}

func TestWriteListJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, writeListJSON(&buf, nil))

	assert.JSONEq(t, `[]`, buf.String())
}

func TestAvailableLabel(t *testing.T) {
	assert.Equal(t, "yes", availableLabel(directory.Integration{Available: true}))
	assert.Equal(t, "no (not yet released)", availableLabel(directory.Integration{AvailabilityReason: "not yet released"}))
	assert.Equal(t, "no", availableLabel(directory.Integration{}))
}

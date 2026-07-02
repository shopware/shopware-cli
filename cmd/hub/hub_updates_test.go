package hub

import (
	"testing"

	"github.com/stretchr/testify/assert"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
)

func TestLabelForEvent(t *testing.T) {
	assert.Equal(t, "New releases", labelForEvent("hub:release:created"))
	assert.Equal(t, "Updated releases", labelForEvent("hub:release:updated"))
	assert.Equal(t, "hub:unknown:event", labelForEvent("hub:unknown:event"))
	assert.Equal(t, "", labelForEvent(""))
}

func TestGroupByEvent(t *testing.T) {
	updates := []account_api.HubUpdate{
		{Event: account_api.HubUpdateEvent{Event: "hub:release:created"}, Title: "Plugin A"},
		{Event: account_api.HubUpdateEvent{Event: "hub:release:updated"}, Title: "Plugin B"},
		{Event: account_api.HubUpdateEvent{Event: "hub:release:created"}, Title: "Plugin C"},
	}

	keys, grouped := groupByEvent(updates)

	assert.Len(t, keys, 2)
	assert.Len(t, grouped["hub:release:created"], 2)
	assert.Len(t, grouped["hub:release:updated"], 1)

	// Keys must be sorted by human-readable label.
	// "New releases" < "Updated releases" alphabetically.
	assert.Equal(t, "hub:release:created", keys[0])
	assert.Equal(t, "hub:release:updated", keys[1])
}

func TestGroupByEvent_Empty(t *testing.T) {
	keys, grouped := groupByEvent(nil)

	assert.Empty(t, keys)
	assert.Empty(t, grouped)
}

func TestGroupByEvent_UnknownEvents(t *testing.T) {
	updates := []account_api.HubUpdate{
		{Event: account_api.HubUpdateEvent{Event: "hub:custom:event"}, Title: "X"},
	}

	keys, grouped := groupByEvent(updates)

	assert.ElementsMatch(t, []string{"hub:custom:event"}, keys)
	assert.Len(t, grouped["hub:custom:event"], 1)
}

func TestFormatDate(t *testing.T) {
	assert.Equal(t, "2024-03-15", formatDate("2024-03-15T12:00:00Z"))
	assert.Equal(t, "2024-03-15", formatDate("2024-03-15"))
	assert.Equal(t, "", formatDate(""))
}

func TestHyperlink_NonTTY(t *testing.T) {
	// In a test environment stdout is not a TTY, so the URL is appended.
	result := hyperlink("Click me", "https://example.com")
	assert.Equal(t, "Click me (https://example.com)", result)
}

func TestHyperlink_EmptyURL(t *testing.T) {
	result := hyperlink("No link", "")
	assert.Equal(t, "No link", result)
}

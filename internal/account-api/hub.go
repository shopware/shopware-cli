package account_api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// HubUpdateEvent holds the event metadata returned by the Hub updates API.
type HubUpdateEvent struct {
	Event     string `json:"event"`
	CreatedAt string `json:"createdAt"`
}

// HubUpdate represents a single item from the Hub updates feed.
type HubUpdate struct {
	Event HubUpdateEvent `json:"event"`
	Title string         `json:"title"`
	Link  string         `json:"link"`
}

// FetchHubUpdates retrieves the latest items from the Hub updates feed.
func (c *Client) FetchHubUpdates(ctx context.Context) ([]HubUpdate, error) {
	r, err := c.NewAuthenticatedRequest(ctx, http.MethodGet, fmt.Sprintf("%s/hub/updates", getApiUrl()), nil)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(r)
	if err != nil {
		return nil, fmt.Errorf("hub updates: %w", err)
	}

	var updates []HubUpdate
	if err := json.Unmarshal(body, &updates); err != nil {
		return nil, fmt.Errorf("hub updates: %w", err)
	}

	return updates, nil
}

package hub_api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/shopware/shopware-cli/logging"
)

var hubBaseURL = "https://hub.shopware.com"

func init() {
	if v := os.Getenv("SHOPWARE_CLI_HUB_URL"); v != "" {
		hubBaseURL = v
	}
}

// HubUpdateEvent holds the event metadata returned by the Community Hub API.
type HubUpdateEvent struct {
	Event     string `json:"event"`
	CreatedAt string `json:"createdAt"`
}

// HubUpdate represents a single item from the Community Hub updates feed.
type HubUpdate struct {
	Event HubUpdateEvent `json:"event"`
	Title string         `json:"title"`
	Link  string         `json:"link"`
}

// FetchUpdates retrieves the latest items from the Community Hub updates feed.
// The endpoint does not require authentication.
func FetchUpdates(ctx context.Context) ([]HubUpdate, error) {
	url := fmt.Sprintf("%s/api/updates", hubBaseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("hub updates: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hub updates: %w", err)
	}
	logging.FromContext(ctx).Debugf("GET %s took %s", url, time.Since(start))

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("hub updates: close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hub updates: API returned status %d: %s", resp.StatusCode, string(body))
	}

	var updates []HubUpdate
	if err := json.NewDecoder(resp.Body).Decode(&updates); err != nil {
		return nil, fmt.Errorf("hub updates: decode response: %w", err)
	}

	return updates, nil
}

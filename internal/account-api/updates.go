package account_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/shopware/shopware-cli/logging"
)

type UpdateCheckExtension struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type UpdateCheckExtensionCompatibility struct {
	Name     string                                  `json:"name"`
	Label    string                                  `json:"label"`
	IconPath string                                  `json:"iconPath"`
	Status   UpdateCheckExtensionCompatibilityStatus `json:"status"`
}

type UpdateCheckExtensionCompatibilityStatus struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

func GetFutureExtensionUpdates(ctx context.Context, currentVersion string, futureVersion string, extensions []UpdateCheckExtension) ([]UpdateCheckExtensionCompatibility, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.shopware.com/swplatform/autoupdate", nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Set("language", "en-GB")
	q.Set("shopwareVersion", currentVersion)
	req.URL.RawQuery = q.Encode()

	bodyBytes, err := json.Marshal(map[string]interface{}{
		"futureShopwareVersion": futureVersion,
		"plugins":               extensions,
	})
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			logging.FromContext(ctx).Errorf("Cannot close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned non-OK status: %d\n%s", resp.StatusCode, string(body))
	}

	var compatibilityResults []UpdateCheckExtensionCompatibility
	if err := json.NewDecoder(resp.Body).Decode(&compatibilityResults); err != nil {
		return nil, err
	}

	return compatibilityResults, nil
}

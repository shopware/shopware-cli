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

// Plugin compatibility status names returned by the store autoupdate
// endpoint. These mirror the constants in Shopware's
// Core\Framework\Update\Services\ExtensionCompatibility. The status `type`
// field is only a display color (green/red/…), so classification must key on
// the semantic `name` instead.
const (
	// CompatibilityCompatible means the installed version already works with
	// the target Shopware version.
	CompatibilityCompatible = "compatible"
	// CompatibilityUpdatableNow / CompatibilityUpdatableFuture mean the
	// extension has a compatible release available ("With new Shopware
	// version"): not a blocker, the constraint just needs to be bumped.
	CompatibilityUpdatableNow    = "updatableNow"
	CompatibilityUpdatableFuture = "updatableFuture"
	// CompatibilityNotCompatible means no compatible successor exists. This
	// is the only genuine blocker.
	CompatibilityNotCompatible = "notCompatible"
	// CompatibilityNotInStore means the extension is not managed by the store.
	CompatibilityNotInStore = "notInStore"
)

type UpdateCheckExtensionCompatibilityStatus struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Type  string `json:"type"`
}

// IsBlocker reports whether this status prevents the upgrade. Only
// notCompatible (no compatible successor) blocks; updatableNow/updatableFuture
// are resolvable by bumping the extension constraint, so they are not blockers.
func (s UpdateCheckExtensionCompatibilityStatus) IsBlocker() bool {
	return s.Name == CompatibilityNotCompatible
}

// IsUpdatable reports whether a compatible release exists that the installed
// version must be bumped to ("With new Shopware version").
func (s UpdateCheckExtensionCompatibilityStatus) IsUpdatable() bool {
	return s.Name == CompatibilityUpdatableNow || s.Name == CompatibilityUpdatableFuture
}

func GetFutureExtensionUpdates(ctx context.Context, currentVersion string, futureVersion string, extensions []UpdateCheckExtension) ([]UpdateCheckExtensionCompatibility, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, getApiUrl()+"/swplatform/autoupdate", nil)
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

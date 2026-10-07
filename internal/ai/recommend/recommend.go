// Package recommend remembers when the "install the Shopware CLI skill"
// recommendation was last shown for an AI client, so it is not repeated too
// often. The state is a cache file, keyed by client.
package recommend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/shopware/shopware-cli/internal/system"
)

// interval is the minimum time between showing the recommendation for a client,
// matching the update notification cadence.
const interval = 24 * time.Hour

func file() string {
	return filepath.Join(system.GetShopwareCliCacheDir(), "ai-recommendations.json")
}

type store struct {
	ShownAt map[string]time.Time `json:"shownAt"`
}

// load reads the store; any problem (missing, corrupt) yields an empty one.
func load() store {
	b, err := os.ReadFile(file())
	if err != nil {
		return store{ShownAt: map[string]time.Time{}}
	}

	var s store
	if json.Unmarshal(b, &s) != nil || s.ShownAt == nil {
		return store{ShownAt: map[string]time.Time{}}
	}

	return s
}

// ShouldShow reports whether the recommendation for client may be shown now (it
// was not shown within the last interval).
func ShouldShow(client string) bool {
	last, ok := load().ShownAt[client]

	return !ok || time.Since(last) >= interval
}

// MarkShown records that the recommendation for client was shown now.
func MarkShown(client string) error {
	s := load()
	s.ShownAt[client] = time.Now()

	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file()), 0o755); err != nil {
		return err
	}

	return os.WriteFile(file(), b, 0o644)
}

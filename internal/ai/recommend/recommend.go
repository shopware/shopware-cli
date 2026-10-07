// Package recommend remembers when the "install the Shopware CLI skill"
// recommendation was last shown for an AI client, so it is not repeated too
// often. The state is a cache file, keyed by client.
package recommend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shopware/shopware-cli/internal/ai/agent"
	"github.com/shopware/shopware-cli/internal/ai/state"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

// interval is the minimum time between showing the recommendation for a client,
// matching the update notification cadence.
const interval = 24 * time.Hour

// guidanceSkill is the bundled skill the recommendation points at.
const guidanceSkill = "shopware-cli"

// Suggest returns the client to record and the line to print, or ("", "") when
// nothing should be shown: no agent detected, the skill already installed for it
// (global or current project), or shown within the suppression window.
func Suggest() (client, msg string) {
	client = agent.Detect()
	if client == "" || installedFor(client) || !ShouldShow(client) {
		return "", ""
	}

	return client, fmt.Sprintf(
		"Official Shopware CLI guidance is available for %s.\nRun: shopware-cli ai add %s --agent %s",
		client, guidanceSkill, client)
}

// installedFor reports whether the guidance skill is recorded for client in the
// global state or the current project's state.
func installedFor(client string) bool {
	if global, err := state.Read(); err == nil && hasSkill(global, client) {
		return true
	}
	if root, err := shop.FindClosestShopwareProject(true); err == nil {
		if project, err := state.ReadProject(root); err == nil && hasSkill(project, client) {
			return true
		}
	}

	return false
}

func hasSkill(f state.File, client string) bool {
	for _, e := range f.Installed {
		if e.Name == guidanceSkill && e.Agent == client {
			return true
		}
	}

	return false
}

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

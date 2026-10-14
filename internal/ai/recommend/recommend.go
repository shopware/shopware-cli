// Package recommend remembers when the "install the Shopware CLI skill"
// recommendation was last shown for an AI client, so it is not repeated too
// often. The state is a cache file, keyed by client.
package recommend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/ai/agent"
	"github.com/shopware/shopware-cli/internal/ai/skills"
	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

// interval is the minimum time between showing the recommendation for a client,
// matching the update notification cadence.
const interval = 24 * time.Hour

// guidanceSkill is the skill the recommendation points at.
const guidanceSkill = "shopware-cli"

// lookupTimeout bounds the best-effort "is it installed?" check so a stalled
// skills.sh never delays the already-finished command.
const lookupTimeout = 5 * time.Second

// listSkills is a seam over skills.List so tests can stub the npx call.
var listSkills = skills.List

// Suggest returns the client to record and the line to print, or ("", "") when
// nothing should be shown: no agent detected, shown within the suppression
// window, or the skill already installed for the client.
//
// The cheap checks run first; the costly "already installed?" check asks
// skills.sh and therefore runs at most once per interval. When it finds the skill
// it records the client as shown, so an installed client is not re-checked on
// every command.
func Suggest(ctx context.Context) (client, msg string) {
	client = agent.Detect()
	if client == "" || !ShouldShow(client) {
		return "", ""
	}

	if installedFor(ctx, client) {
		_ = MarkShown(client)

		return "", ""
	}

	// Describe the install rather than instruct it: a detected agent may run a
	// printed command without asking, and installing without the user's consent
	// is out of scope (#1338). The guidance skill is general, not shop-specific,
	// so point at a global install, which runs from anywhere.
	return client, fmt.Sprintf(
		"Shopware CLI guidance is available for %s.\nTo enable it, ask the user to run: shopware-cli ai add %s --agent %s --global",
		client, guidanceSkill, client)
}

// installedFor reports whether the guidance skill is installed for client,
// according to skills.sh (the global scope or the current project). The lookup is
// best-effort and time-bounded so a stalled skills.sh never blocks the CLI.
func installedFor(ctx context.Context, client string) bool {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	if records, err := listSkills(ctx, "", true); err == nil && hasGuidance(records, client) {
		return true
	}
	if root, err := shop.FindClosestShopwareProject(true); err == nil {
		if records, err := listSkills(ctx, root, false); err == nil && hasGuidance(records, client) {
			return true
		}
	}

	return false
}

func hasGuidance(records []skills.Installed, client string) bool {
	for _, r := range records {
		if r.Name == guidanceSkill && matchesAgent(r.Agents, client) {
			return true
		}
	}

	return false
}

// matchesAgent reports whether the agent id (e.g. "claude-code") is among the
// display names skills.sh returns (e.g. "Claude Code"), compared on alphanumerics.
func matchesAgent(agents []string, id string) bool {
	want := normalizeAgent(id)
	for _, a := range agents {
		if normalizeAgent(a) == want {
			return true
		}
	}

	return false
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func normalizeAgent(s string) string {
	return nonAlnum.ReplaceAllString(strings.ToLower(s), "")
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

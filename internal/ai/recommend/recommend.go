// Package recommend decides when to suggest installing a Shopware skill to a
// detected AI client, and remembers when a recommendation was last shown (a cache
// file) so it is not repeated too often.
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

// interval is the minimum time between showing a recommendation, matching the
// update notification cadence.
const interval = 24 * time.Hour

// lookupTimeout bounds the best-effort "is it installed?" check so a stalled
// skills.sh never delays the already-finished command.
const lookupTimeout = 5 * time.Second

// listSkills is a seam over skills.List so tests can stub the npx call.
var listSkills = skills.List

// Suggest returns the detected client and the line to print recommending skill,
// or ("", "") when nothing should be shown: no agent detected, no skill given,
// shown within the suppression window, or the skill already installed.
//
// The cheap checks run first; the costly "already installed?" check asks skills.sh
// and therefore runs at most once per interval. When it finds the skill it records
// the recommendation as shown, so it is not re-checked on every command.
func Suggest(ctx context.Context, skill string) (client, msg string) {
	client = agent.Detect()
	if client == "" || skill == "" || !ShouldShow() {
		return "", ""
	}

	installed, known := installedFor(ctx, skill, client)
	if !known {
		// The lookup failed (e.g. timed out); skip quietly and retry next run
		// rather than risk a wrong recommendation or burn the 24h window.
		return "", ""
	}
	if installed {
		_ = MarkShown()

		return "", ""
	}

	// Describe the install rather than instruct it: a detected agent may run a
	// printed command without asking, and installing without the user's consent
	// is out of scope (#1338). The guidance skill is general, not shop-specific,
	// so point at a global install, which runs from anywhere.
	return client, fmt.Sprintf(
		"Shopware CLI guidance is available for %s.\nTo enable it, ask the user to run: shopware-cli ai add %s --agent %s --global",
		client, skill, client)
}

// installedFor reports whether skill is installed for client and whether the
// lookup could determine it. A failed or timed-out skills.sh lookup returns
// known=false, so the caller does not mistake "couldn't check" for "absent". The
// lookup is time-bounded so a stalled skills.sh never blocks the CLI.
func installedFor(ctx context.Context, skill, client string) (installed, known bool) {
	ctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()

	global, err := listSkills(ctx, "", true)
	if err != nil {
		return false, false
	}
	if hasSkill(global, skill, client) {
		return true, true
	}

	root, err := shop.FindClosestShopwareProject(true)
	if err != nil {
		// No project to check; the global lookup was conclusive.
		return false, true
	}
	project, err := listSkills(ctx, root, false)
	if err != nil {
		return false, false
	}

	return hasSkill(project, skill, client), true
}

func hasSkill(records []skills.Installed, skill, client string) bool {
	for _, r := range records {
		if r.Name == skill && matchesAgent(r.Agents, client) {
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
	LastShownAt time.Time `json:"lastShownAt"`
}

// load reads the store; any problem (missing, corrupt) yields the zero store,
// whose zero LastShownAt means "never shown".
func load() store {
	b, err := os.ReadFile(file())
	if err != nil {
		return store{}
	}

	var s store
	if json.Unmarshal(b, &s) != nil {
		return store{}
	}

	return s
}

// ShouldShow reports whether a recommendation may be shown now (none was shown
// within the last interval, regardless of client).
func ShouldShow() bool {
	return time.Since(load().LastShownAt) >= interval
}

// MarkShown records that a recommendation was shown now.
func MarkShown() error {
	b, err := json.MarshalIndent(store{LastShownAt: time.Now()}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(file()), 0o755); err != nil {
		return err
	}

	return os.WriteFile(file(), b, 0o644)
}

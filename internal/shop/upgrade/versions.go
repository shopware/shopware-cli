package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/logging"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// releaseCycle mirrors one entry of the endoflife.date API response. The
// support and eol fields are either an ISO date string or a boolean.
type releaseCycle struct {
	Cycle   string `json:"cycle"`
	Support any    `json:"support"`
	EOL     any    `json:"eol"`
}

type supportWindow struct {
	activeUntil time.Time
	eolUntil    time.Time
}

// LoadCatalog fetches all Shopware versions newer than current and annotates
// them with release-cycle support information. Stable versions only — release
// candidates and other prereleases are excluded, matching `project
// upgrade-check`.
func (u *ProjectUpgrader) LoadCatalog(ctx context.Context, current *version.Version) (*Catalog, error) {
	all, err := u.shopwareVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("load shopware versions: %w", err)
	}

	windows := loadSupportWindows(ctx, u.endOfLifeURL)

	candidates := make([]*version.Version, 0, len(all))
	for _, raw := range all {
		v, err := version.NewVersion(raw)
		if err != nil || v.IsPrerelease() || strings.Contains(strings.ToUpper(raw), "RC") {
			continue
		}
		if !v.GreaterThan(current) {
			continue
		}
		candidates = append(candidates, v)
	}

	sort.Sort(sort.Reverse(version.Collection(candidates)))
	candidates = slices.CompactFunc(candidates, func(a, b *version.Version) bool {
		return a.Equal(b)
	})

	catalog := &Catalog{Current: current, Recommended: -1, LatestPatch: -1}
	now := time.Now()

	currentCycle := cycleOf(current)
	for i, v := range candidates {
		opt := VersionOption{
			Version:         v,
			ReleaseNotesURL: "https://github.com/shopware/shopware/releases/tag/v" + v.String(),
		}

		if w, ok := windows[cycleOf(v)]; ok {
			opt.SupportUntil = w.eolUntil
			switch {
			case now.Before(w.activeUntil):
				opt.SupportType = "active"
			case now.Before(w.eolUntil):
				opt.SupportType = "security"
			case !w.eolUntil.IsZero():
				opt.SupportType = "eol"
			}
		}

		if catalog.LatestPatch == -1 && cycleOf(v) == currentCycle {
			catalog.LatestPatch = i
			opt.Tag = "latest " + currentCycle + " patch"
		}

		catalog.Options = append(catalog.Options, opt)
	}

	// Recommended: the newest stable version.
	if len(catalog.Options) > 0 {
		catalog.Recommended = 0
		catalog.Options[0].Tag = "recommended"
	}

	return catalog, nil
}

// cycleOf maps a Shopware version to its release cycle key, e.g. 6.6.10.3 -> "6.6".
func cycleOf(v *version.Version) string {
	segments := v.Segments()
	if len(segments) < 2 {
		return v.String()
	}
	return fmt.Sprintf("%d.%d", segments[0], segments[1])
}

// loadSupportWindows fetches the release-cycle table. Failures only degrade
// the display (no support columns), so they are logged and swallowed.
func loadSupportWindows(ctx context.Context, endOfLifeURL string) map[string]supportWindow {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endOfLifeURL, http.NoBody)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "shopware-cli")

	resp, err := httpClient.Do(req)
	if err != nil {
		logging.FromContext(ctx).Debugf("release cycle lookup failed: %v", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		logging.FromContext(ctx).Debugf("release cycle lookup returned status %d", resp.StatusCode)
		return nil
	}

	var cycles []releaseCycle
	if err := json.NewDecoder(resp.Body).Decode(&cycles); err != nil {
		logging.FromContext(ctx).Debugf("release cycle response invalid: %v", err)
		return nil
	}

	windows := make(map[string]supportWindow, len(cycles))
	for _, c := range cycles {
		windows[c.Cycle] = supportWindow{
			activeUntil: parseCycleDate(c.Support),
			eolUntil:    parseCycleDate(c.EOL),
		}
	}
	return windows
}

// parseCycleDate converts an endoflife.date support/eol field ("2026-07-01",
// true, or false) into a time. Booleans carry no date, so they map to zero.
func parseCycleDate(v any) time.Time {
	s, ok := v.(string)
	if !ok {
		return time.Time{}
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// supportLeft formats the distance between now and until as "1y 7m", "7m", or
// "" when until is unknown or already passed.
func supportLeft(until, now time.Time) string {
	if until.IsZero() || !until.After(now) {
		return ""
	}

	months := 0
	cursor := now
	for {
		next := cursor.AddDate(0, 1, 0)
		if next.After(until) {
			break
		}
		months++
		cursor = next
	}

	years := months / 12
	months %= 12
	switch {
	case years > 0 && months > 0:
		return fmt.Sprintf("%dy %dm", years, months)
	case years > 0:
		return fmt.Sprintf("%dy", years)
	case months > 0:
		return fmt.Sprintf("%dm", months)
	}
	return "<1m"
}

// PackagistReachable reports whether the Packagist repository answers,
// i.e. whether Composer will be able to download packages.
func (u *ProjectUpgrader) PackagistReachable(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.packagistPingURL, http.NoBody)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "shopware-cli")

	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK
}

// IsMultiMajorJump reports whether upgrading from current to target skips
// more than one major.minor release line (e.g. 6.5 -> 6.7). Extension vendors
// typically validate one line at a time, so such paths carry extra risk.
func IsMultiMajorJump(current, target *version.Version) bool {
	c := current.Segments()
	t := target.Segments()
	if len(c) < 2 || len(t) < 2 {
		return false
	}
	if t[0] != c[0] {
		return true
	}
	return t[1]-c[1] > 1
}

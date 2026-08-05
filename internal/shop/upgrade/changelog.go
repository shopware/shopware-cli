package upgrade

import (
	"context"
	"html"
	"sort"
	"strings"

	"github.com/shyim/go-version"

	"github.com/shopware/shopware-cli/logging"
)

// ExtensionChangelog collects the Store changelog entries an extension update
// applies — everything after the installed release up to and including the
// release the upgrade moves to.
type ExtensionChangelog struct {
	// Extension is the technical name, e.g. SwagPayPal.
	Extension string
	// From is the installed version, To the update's target version.
	From string
	To   string
	// Entries are ordered newest first.
	Entries []ChangelogEntry
}

// ChangelogEntry is one release note of an extension update.
type ChangelogEntry struct {
	Version string
	// Date is the release date (yyyy-mm-dd), "" when the Store has none.
	Date string
	// Text is the release note as plain text (the Store serves HTML).
	Text string
}

// LoadExtensionChangelogs fetches the Store changelogs for every extension
// the upgrade updates (installed -> available release) and keeps the entries
// inside that version window. Store metadata is advisory, so failures degrade
// to nil instead of erroring.
func (u *ProjectUpgrader) LoadExtensionChangelogs(ctx context.Context, target string, results []ExtensionResult) []ExtensionChangelog {
	type window struct{ from, to *version.Version }
	updates := make(map[string]window)
	names := make([]string, 0, len(results))
	for _, res := range results {
		if res.Available == "" || res.Available == res.Extension.Version {
			continue
		}
		from, fromErr := version.NewVersion(res.Extension.Version)
		to, toErr := version.NewVersion(res.Available)
		if fromErr != nil || toErr != nil || !to.GreaterThan(from) {
			continue
		}
		updates[res.Extension.Name] = window{from: from, to: to}
		names = append(names, res.Extension.Name)
	}
	if len(names) == 0 {
		return nil
	}

	plugins, err := u.storePlugins(ctx, "en_GB", target, names)
	if err != nil {
		logging.FromContext(ctx).Debugf("store changelog lookup failed: %v", err)
		return nil
	}

	var changelogs []ExtensionChangelog
	for _, plugin := range plugins {
		win, ok := updates[plugin.Name]
		if !ok {
			continue
		}

		cl := ExtensionChangelog{
			Extension: plugin.Name,
			From:      win.from.String(),
			To:        win.to.String(),
		}
		for _, entry := range plugin.Changelogs {
			v, err := version.NewVersion(entry.Version)
			if err != nil || !v.GreaterThan(win.from) || v.GreaterThan(win.to) {
				continue
			}
			cl.Entries = append(cl.Entries, ChangelogEntry{
				Version: entry.Version,
				Date:    changelogDate(entry.CreationDate.Date),
				Text:    htmlToText(entry.Text),
			})
		}
		if len(cl.Entries) == 0 {
			continue
		}

		sort.SliceStable(cl.Entries, func(i, j int) bool {
			vi, ei := version.NewVersion(cl.Entries[i].Version)
			vj, ej := version.NewVersion(cl.Entries[j].Version)
			if ei != nil || ej != nil {
				return cl.Entries[i].Version > cl.Entries[j].Version
			}
			return vi.GreaterThan(vj)
		})
		changelogs = append(changelogs, cl)
	}

	sort.Slice(changelogs, func(i, j int) bool { return changelogs[i].Extension < changelogs[j].Extension })
	return changelogs
}

// changelogDate reduces the Store's timestamp ("2026-01-15 00:00:00.000000")
// to its date part.
func changelogDate(date string) string {
	if len(date) >= 10 {
		return date[:10]
	}
	return date
}

// htmlToText flattens the Store's HTML release notes to plain text: list
// items become bullet lines, block-level closings become line breaks, all
// remaining tags are dropped, and entities are unescaped.
func htmlToText(s string) string {
	replacer := strings.NewReplacer(
		"<li>", "- ",
		"</li>", "\n",
		"</p>", "\n",
		"<br>", "\n",
		"<br/>", "\n",
		"<br />", "\n",
	)
	s = replacer.Replace(s)

	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}

	lines := strings.Split(html.UnescapeString(b.String()), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}

package upgrade

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
)

func updateResult(name, installed, available string) ExtensionResult {
	return ExtensionResult{
		Extension: InstalledExtension{Name: name, Version: installed, ComposerManaged: true},
		Status:    ExtOK,
		Available: available,
	}
}

func storeChangelog(v, date, text string) account_api.StoreChangelog {
	return account_api.StoreChangelog{Version: v, Text: text, CreationDate: account_api.StoreDate{Date: date}}
}

func TestLoadExtensionChangelogs(t *testing.T) {
	dir := setupProject(t)
	u := newTestUpgrader(t, dir)

	var gotNames []string
	var gotVersion string
	u.storePlugins = func(_ context.Context, locale, shopwareVersion string, names []string) ([]account_api.StorePlugin, error) {
		assert.Equal(t, "en_GB", locale)
		gotVersion = shopwareVersion
		gotNames = names
		return []account_api.StorePlugin{{
			Name: "SwagDemo",
			Changelogs: []account_api.StoreChangelog{
				storeChangelog("2.5.0", "2026-06-01 00:00:00.000000", "too new"),
				storeChangelog("2.1.0", "2026-03-01 00:00:00.000000", "<p>Adds <b>things</b></p><ul><li>one</li><li>two &amp; three</li></ul>"),
				storeChangelog("2.0.0", "2026-01-15 00:00:00.000000", "Major rework"),
				storeChangelog("1.0.0", "2025-01-01 00:00:00.000000", "already installed"),
			},
		}}, nil
	}

	results := []ExtensionResult{
		updateResult("SwagDemo", "1.0.0", "2.1.0"),
		updateResult("NoUpdate", "3.0.0", "3.0.0"),
		{Extension: InstalledExtension{Name: "NoRelease", Version: "1.0.0"}, Status: ExtBlocked},
	}

	changelogs := u.LoadExtensionChangelogs(t.Context(), "6.7.11.0", results)

	assert.Equal(t, []string{"SwagDemo"}, gotNames, "only extensions with an actual update are queried")
	assert.Equal(t, "6.7.11.0", gotVersion)

	require.Len(t, changelogs, 1)
	cl := changelogs[0]
	assert.Equal(t, "SwagDemo", cl.Extension)
	assert.Equal(t, "1.0.0", cl.From)
	assert.Equal(t, "2.1.0", cl.To)

	require.Len(t, cl.Entries, 2, "entries outside (installed, available] are dropped")
	assert.Equal(t, "2.1.0", cl.Entries[0].Version, "newest first")
	assert.Equal(t, "2026-03-01", cl.Entries[0].Date)
	assert.Equal(t, "Adds things\n- one\n- two & three", cl.Entries[0].Text, "HTML is flattened to text")
	assert.Equal(t, "2.0.0", cl.Entries[1].Version)
}

func TestLoadExtensionChangelogsNoUpdatesSkipsStore(t *testing.T) {
	dir := setupProject(t)
	u := newTestUpgrader(t, dir)
	u.storePlugins = func(context.Context, string, string, []string) ([]account_api.StorePlugin, error) {
		t.Fatal("the store must not be queried without updates")
		return nil, context.Canceled
	}

	assert.Nil(t, u.LoadExtensionChangelogs(t.Context(), "6.7.11.0", []ExtensionResult{
		updateResult("NoUpdate", "3.0.0", "3.0.0"),
	}))
}

func TestLoadExtensionChangelogsStoreErrorIsAdvisory(t *testing.T) {
	dir := setupProject(t)
	u := newTestUpgrader(t, dir)
	u.storePlugins = func(context.Context, string, string, []string) ([]account_api.StorePlugin, error) {
		return nil, errors.New("store down")
	}

	assert.Nil(t, u.LoadExtensionChangelogs(t.Context(), "6.7.11.0", []ExtensionResult{
		updateResult("SwagDemo", "1.0.0", "2.0.0"),
	}))
}

func TestReportIncludesChangelogs(t *testing.T) {
	report := renderReport(ReportData{
		ProjectName: "acme-shop",
		Current:     "6.6.10.3",
		Target:      "6.7.11.0",
		Changelogs: []ExtensionChangelog{{
			Extension: "SwagDemo",
			From:      "1.0.0",
			To:        "2.0.0",
			Entries: []ChangelogEntry{
				{Version: "2.0.0", Date: "2026-01-15", Text: "Major rework"},
			},
		}},
	})
	assert.Contains(t, report, "## Extension update changelogs")
	assert.Contains(t, report, "### SwagDemo (1.0.0 → 2.0.0)")
	assert.Contains(t, report, "**2.0.0 — 2026-01-15**")
	assert.Contains(t, report, "Major rework")
}

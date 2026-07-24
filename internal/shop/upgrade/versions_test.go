package upgrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCatalog(t *testing.T) {
	u := NewProjectUpgrader(t.TempDir(), nil)
	u.shopwareVersions = func(ctx context.Context) ([]string, error) {
		return []string{
			"6.5.8.0", "6.6.9.0", "6.6.10.2", "6.6.10.3", "6.6.10.19",
			"6.7.0.0-rc1", "6.7.10.0", "6.7.11.0",
		}, nil
	}

	activeUntil := time.Now().AddDate(1, 0, 0).Format("2006-01-02")
	eolUntil := time.Now().AddDate(1, 7, 0).Format("2006-01-02")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"cycle": "6.7", "support": "` + activeUntil + `", "eol": "` + eolUntil + `"},
			{"cycle": "6.6", "support": false, "eol": "` + eolUntil + `"}
		]`))
	}))
	defer server.Close()
	u.endOfLifeURL = server.URL

	current := version.Must(version.NewVersion("6.6.10.3"))
	catalog, err := u.LoadCatalog(t.Context(), current)
	require.NoError(t, err)

	got := make([]string, 0, len(catalog.Options))
	for _, o := range catalog.Options {
		got = append(got, o.Version.String())
	}
	// Newer than current only, no prereleases, newest first.
	assert.Equal(t, []string{"6.7.11.0", "6.7.10.0", "6.6.10.19"}, got)

	assert.Equal(t, 0, catalog.Recommended)
	assert.Equal(t, "recommended", catalog.Options[0].Tag)
	assert.Equal(t, 2, catalog.LatestPatch)
	assert.Equal(t, "latest 6.6 patch", catalog.Options[2].Tag)

	assert.Equal(t, "active", catalog.Options[0].SupportType)
	assert.Equal(t, "security", catalog.Options[2].SupportType, "6.6 support date is false -> security until EOL")
	assert.NotEmpty(t, catalog.Options[0].SupportLeft())
	assert.Equal(t, "https://github.com/shopware/shopware/releases/tag/v6.7.11.0", catalog.Options[0].ReleaseNotesURL)
}

func TestLoadCatalogWithoutSupportData(t *testing.T) {
	u := NewProjectUpgrader(t.TempDir(), nil)
	u.shopwareVersions = func(ctx context.Context) ([]string, error) {
		return []string{"6.7.11.0"}, nil
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	u.endOfLifeURL = server.URL

	catalog, err := u.LoadCatalog(t.Context(), version.Must(version.NewVersion("6.6.10.3")))
	require.NoError(t, err)
	require.Len(t, catalog.Options, 1)
	assert.Empty(t, catalog.Options[0].SupportType, "support columns degrade gracefully")
	assert.Empty(t, catalog.Options[0].SupportLeft())
}

func TestSupportLeft(t *testing.T) {
	now := time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC)

	assert.Equal(t, "", supportLeft(time.Time{}, now))
	assert.Equal(t, "", supportLeft(now.AddDate(0, 0, -1), now))
	assert.Equal(t, "<1m", supportLeft(now.AddDate(0, 0, 10), now))
	assert.Equal(t, "7m", supportLeft(now.AddDate(0, 7, 3), now))
	assert.Equal(t, "1y 7m", supportLeft(now.AddDate(1, 7, 3), now))
	assert.Equal(t, "2y", supportLeft(now.AddDate(2, 0, 5), now))
}

func TestCycleOf(t *testing.T) {
	assert.Equal(t, "6.6", cycleOf(version.Must(version.NewVersion("6.6.10.3"))))
	assert.Equal(t, "6.7", cycleOf(version.Must(version.NewVersion("6.7.0.0"))))
}

func TestIsMultiMajorJump(t *testing.T) {
	v := func(s string) *version.Version { return version.Must(version.NewVersion(s)) }

	assert.False(t, IsMultiMajorJump(v("6.6.10.3"), v("6.6.10.19")))
	assert.False(t, IsMultiMajorJump(v("6.6.10.3"), v("6.7.11.0")))
	assert.True(t, IsMultiMajorJump(v("6.5.8.0"), v("6.7.11.0")))
	assert.True(t, IsMultiMajorJump(v("6.6.10.3"), v("7.0.0.0")))
}

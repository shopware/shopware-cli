package account_api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetStorePluginsByName(t *testing.T) {
	var gotQuery map[string][]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/pluginStore/pluginsByName", r.URL.Path)
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`[{
			"id": 42,
			"name": "SwagDemo",
			"label": "Demo Plugin",
			"description": "<p>Long description</p>",
			"version": "2.1.0",
			"ratingAverage": 4.5,
			"link": "http://store.shopware.com:80/swag-demo.html",
			"iconPath": "https://store.shopware.com/icon.png",
			"releaseDate": {"date": "2026-01-15 00:00:00.000000"},
			"producer": {"name": "shopware AG", "website": "https://shopware.com"},
			"infos": [{"shortDescription": "Short text"}],
			"pictures": [
				{"remoteLink": "https://img/1.png", "preview": true, "priority": 1},
				{"remoteLink": "", "preview": false, "priority": 2}
			],
			"changelog": [{"version": "2.1.0", "text": "Fixes", "creationDate": {"date": "2026-01-15"}}]
		}]`))
	}))
	defer srv.Close()
	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", srv.URL)

	plugins, err := GetStorePluginsByName(t.Context(), "en_GB", "6.6.10.3", []string{"SwagDemo", "SwagOther"})
	require.NoError(t, err)

	assert.Equal(t, []string{"en_GB"}, gotQuery["locale"])
	assert.Equal(t, []string{"6.6.10.3"}, gotQuery["shopwareVersion"])
	assert.Equal(t, []string{"SwagDemo", "SwagOther"}, gotQuery["technicalNames[]"])

	require.Len(t, plugins, 1)
	p := plugins[0]
	assert.Equal(t, 42, p.ID)
	assert.Equal(t, "Demo Plugin", p.Label)
	assert.Equal(t, "2.1.0", p.Version)
	assert.InDelta(t, 4.5, p.RatingAverage, 0.001)
	assert.Equal(t, "https://store.shopware.com/swag-demo.html", p.StoreLink, "explicit-port links are normalized")
	assert.Equal(t, "Short text", p.ShortDescription)
	assert.Equal(t, "shopware AG", p.ProducerName)
	assert.Equal(t, "2026-01-15 00:00:00.000000", p.ReleaseDate)
	require.Len(t, p.Pictures, 1, "pictures without a remote link are dropped")
	assert.Equal(t, "https://img/1.png", p.Pictures[0].URL)
	require.Len(t, p.Changelogs, 1)
	assert.Equal(t, "Fixes", p.Changelogs[0].Text)
}

func TestGetStorePluginsByNameErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()
	t.Setenv("SHOPWARE_CLI_API_ENDPOINT", srv.URL)

	_, err := GetStorePluginsByName(t.Context(), "en_GB", "6.6.10.3", []string{"SwagDemo"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "502")
}

func TestNormalizeStoreLink(t *testing.T) {
	assert.Equal(t, "https://store.shopware.com/a", normalizeStoreLink("http://store.shopware.com:80/a"))
	assert.Equal(t, "https://store.shopware.com/a", normalizeStoreLink("https://store.shopware.com:443/a"))
	assert.Equal(t, "https://store.shopware.com/a", normalizeStoreLink("http://store.shopware.com/a"))
	assert.Equal(t, "https://store.shopware.com", normalizeStoreLink("http://store.shopware.com:80"))
	assert.Equal(t, "https://example.com/a", normalizeStoreLink("https://example.com/a"))
	assert.Equal(t, "http://store.shopware.com:8080/a", normalizeStoreLink("http://store.shopware.com:8080/a"),
		"custom ports are not default-port URLs and must pass through untouched")
}

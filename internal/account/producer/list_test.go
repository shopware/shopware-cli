package producer

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	accountApi "github.com/shopware/shopware-cli/internal/account-api"
)

func testExtension(name, generation, status string) accountApi.Extension {
	var ext accountApi.Extension
	ext.Name = name
	ext.Generation.Name = generation
	ext.Generation.Description = generation
	ext.Status.Name = status
	ext.Producer.Name = "Acme"
	ext.Producer.Id = 1
	return ext
}

func TestFilterExtensions(t *testing.T) {
	extensions := []accountApi.Extension{
		testExtension("PaymentPlugin", accountApi.ExtensionGenerationPlatform, "approved"),
		testExtension("ShippingApp", accountApi.ExtensionGenerationApps, "instore"),
		testExtension("Legacy", accountApi.ExtensionGenerationClassic, "approved"),
		testExtension("Gone", accountApi.ExtensionGenerationPlatform, "deleted"),
		testExtension("", accountApi.ExtensionGenerationPlatform, "approved"),
		testExtension("ThemePlugin", accountApi.ExtensionGenerationPlatform, "incomplete"),
	}

	t.Run("no type filter keeps plugins and apps", func(t *testing.T) {
		got := filterExtensions(extensions, false, false)
		assert.ElementsMatch(t, []string{"PaymentPlugin", "ShippingApp", "ThemePlugin"}, extensionNames(got))
	})

	t.Run("plugin filter", func(t *testing.T) {
		got := filterExtensions(extensions, true, false)
		assert.ElementsMatch(t, []string{"PaymentPlugin", "ThemePlugin"}, extensionNames(got))
	})

	t.Run("app filter", func(t *testing.T) {
		got := filterExtensions(extensions, false, true)
		assert.ElementsMatch(t, []string{"ShippingApp"}, extensionNames(got))
	})
}

func TestIncludeExtension(t *testing.T) {
	assert.True(t, includeExtension(testExtension("A", accountApi.ExtensionGenerationPlatform, "approved"), false, false))
	assert.False(t, includeExtension(testExtension("A", accountApi.ExtensionGenerationClassic, "approved"), false, false))
	assert.False(t, includeExtension(testExtension("A", accountApi.ExtensionGenerationPlatform, "deleted"), false, false))
	assert.False(t, includeExtension(testExtension("", accountApi.ExtensionGenerationPlatform, "approved"), false, false))
	assert.False(t, includeExtension(testExtension("A", accountApi.ExtensionGenerationApps, "approved"), true, false))
	assert.True(t, includeExtension(testExtension("A", accountApi.ExtensionGenerationApps, "approved"), false, true))
	assert.True(t, includeExtension(testExtension("A", accountApi.ExtensionGenerationPlatform, "approved"), true, false))
}

func extensionNames(extensions []accountApi.Extension) []string {
	names := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		names = append(names, ext.Name)
	}
	return names
}

func TestListExtensionsJSON(t *testing.T) {
	api := &fakeListAPI{
		extensions: []accountApi.Extension{
			testExtension("PaymentPlugin", accountApi.ExtensionGenerationPlatform, "approved"),
			testExtension("Legacy", accountApi.ExtensionGenerationClassic, "approved"),
			testExtension("AwesomeApp", accountApi.ExtensionGenerationApps, "instore"),
		},
	}

	var buf bytes.Buffer
	require.NoError(t, ListExtensions(t.Context(), api, &buf, ListOptions{JSON: true, Search: "acme"}))

	require.NotNil(t, api.criteria)
	assert.Equal(t, "acme", api.criteria.Search)
	assert.Equal(t, "name", api.criteria.OrderBy)
	assert.Equal(t, "asc", api.criteria.OrderSequence)
	assert.Equal(t, 100, api.criteria.Limit)

	var items []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 2)
	assert.Equal(t, "AwesomeApp", items[0]["name"])
	assert.Equal(t, "PaymentPlugin", items[1]["name"])
	assert.Equal(t, "Acme", items[0]["producer"])
}

func TestListExtensionsTable(t *testing.T) {
	api := &fakeListAPI{
		extensions: []accountApi.Extension{
			testExtension("PaymentPlugin", accountApi.ExtensionGenerationPlatform, "approved"),
		},
	}

	var buf bytes.Buffer
	require.NoError(t, ListExtensions(t.Context(), api, &buf, ListOptions{}))

	require.NotNil(t, api.criteria)
	assert.Empty(t, api.criteria.Search)
	assert.Contains(t, buf.String(), "PaymentPlugin")
	assert.Contains(t, buf.String(), "Acme")
}

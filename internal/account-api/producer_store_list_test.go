package account_api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testExtension(name, generation, status string) Extension {
	var ext Extension
	ext.Name = name
	ext.Generation.Name = generation
	ext.Generation.Description = generation
	ext.Status.Name = status
	ext.Producer.Name = "Acme"
	ext.Producer.Id = 1
	return ext
}

func TestFilterExtensions(t *testing.T) {
	extensions := []Extension{
		testExtension("PaymentPlugin", ExtensionGenerationPlatform, "approved"),
		testExtension("ShippingApp", ExtensionGenerationApps, "instore"),
		testExtension("Legacy", ExtensionGenerationClassic, "approved"),
		testExtension("Gone", ExtensionGenerationPlatform, "deleted"),
		testExtension("", ExtensionGenerationPlatform, "approved"),
		testExtension("ThemePlugin", ExtensionGenerationPlatform, "incomplete"),
	}

	t.Run("no type filter keeps plugins and apps", func(t *testing.T) {
		got := FilterExtensions(extensions, false, false)
		assert.ElementsMatch(t, []string{"PaymentPlugin", "ShippingApp", "ThemePlugin"}, extensionNames(got))
	})

	t.Run("plugin filter", func(t *testing.T) {
		got := FilterExtensions(extensions, true, false)
		assert.ElementsMatch(t, []string{"PaymentPlugin", "ThemePlugin"}, extensionNames(got))
	})

	t.Run("app filter", func(t *testing.T) {
		got := FilterExtensions(extensions, false, true)
		assert.ElementsMatch(t, []string{"ShippingApp"}, extensionNames(got))
	})
}

func TestIncludeExtension(t *testing.T) {
	assert.True(t, IncludeExtension(testExtension("A", ExtensionGenerationPlatform, "approved"), false, false))
	assert.False(t, IncludeExtension(testExtension("A", ExtensionGenerationClassic, "approved"), false, false))
	assert.False(t, IncludeExtension(testExtension("A", ExtensionGenerationPlatform, "deleted"), false, false))
	assert.False(t, IncludeExtension(testExtension("", ExtensionGenerationPlatform, "approved"), false, false))
	assert.False(t, IncludeExtension(testExtension("A", ExtensionGenerationApps, "approved"), true, false))
	assert.True(t, IncludeExtension(testExtension("A", ExtensionGenerationApps, "approved"), false, true))
	assert.True(t, IncludeExtension(testExtension("A", ExtensionGenerationPlatform, "approved"), true, false))
}

func TestListProducerExtensions(t *testing.T) {
	producerB := func(name string) Extension {
		ext := testExtension(name, ExtensionGenerationPlatform, "approved")
		ext.Producer.Name = "Beta"
		ext.Producer.Id = 2
		return ext
	}

	var gotCriteria *ListExtensionCriteria
	producer := &fakeProducer{
		extensionsFn: func(_ context.Context, criteria *ListExtensionCriteria) ([]Extension, error) {
			gotCriteria = criteria
			return []Extension{
				producerB("Zebra"),
				testExtension("Alpha", ExtensionGenerationPlatform, "approved"),
				testExtension("Legacy", ExtensionGenerationClassic, "approved"),
			}, nil
		},
	}

	extensions, err := ListProducerExtensions(t.Context(), producer, ListExtensionOptions{Search: "shop"})

	require.NoError(t, err)
	require.NotNil(t, gotCriteria)
	assert.Equal(t, 100, gotCriteria.Limit)
	assert.Equal(t, "shop", gotCriteria.Search)
	assert.Equal(t, "name", gotCriteria.OrderBy)
	assert.Equal(t, "asc", gotCriteria.OrderSequence)
	// filtered and sorted by producer name, then extension name
	assert.Equal(t, []string{"Alpha", "Zebra"}, extensionNames(extensions))
}

func TestListProducerExtensionsPaginates(t *testing.T) {
	fullPage := make([]Extension, 0, 100)
	for i := range 100 {
		fullPage = append(fullPage, testExtension(fmt.Sprintf("Ext%03d", i), ExtensionGenerationPlatform, "approved"))
	}

	var offsets []int
	producer := &fakeProducer{
		extensionsFn: func(_ context.Context, criteria *ListExtensionCriteria) ([]Extension, error) {
			offsets = append(offsets, criteria.Offset)
			switch criteria.Offset {
			case 0:
				return fullPage, nil
			case 100:
				return fullPage[:30], nil
			default:
				return nil, nil
			}
		},
	}

	extensions, err := ListProducerExtensions(t.Context(), producer, ListExtensionOptions{})

	require.NoError(t, err)
	assert.Len(t, extensions, 130)
	assert.Equal(t, []int{0, 100}, offsets)
}

func TestWriteExtensionsJSON(t *testing.T) {
	ext := testExtension("PaymentPlugin", ExtensionGenerationPlatform, "approved")
	ext.IsCompatibleWithLatestShopwareVersion = true

	var buf bytes.Buffer
	require.NoError(t, WriteExtensionsJSON(&buf, []Extension{ext}))

	var items []struct {
		Name       string `json:"name"`
		Type       string `json:"type"`
		Compatible bool   `json:"compatibleWithLatestVersion"`
		Status     string `json:"status"`
		Producer   string `json:"producer"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &items))
	require.Len(t, items, 1)
	assert.Equal(t, "PaymentPlugin", items[0].Name)
	assert.Equal(t, ExtensionGenerationPlatform, items[0].Type)
	assert.True(t, items[0].Compatible)
	assert.Equal(t, "approved", items[0].Status)
	assert.Equal(t, "Acme", items[0].Producer)
}

func TestWriteExtensionsTable(t *testing.T) {
	ext := testExtension("PaymentPlugin", ExtensionGenerationPlatform, "approved")
	ext.IsCompatibleWithLatestShopwareVersion = true

	var buf bytes.Buffer
	require.NoError(t, WriteExtensionsTable(&buf, []Extension{ext}))

	output := buf.String()
	assert.Contains(t, output, "Acme")
	assert.Contains(t, output, "PaymentPlugin")
	assert.Contains(t, output, "Yes")
}

func extensionNames(extensions []Extension) []string {
	names := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		names = append(names, ext.Name)
	}
	return names
}

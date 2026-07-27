package account

import (
	"testing"

	"github.com/stretchr/testify/assert"

	account_api "github.com/shopware/shopware-cli/internal/account-api"
)

func testExtension(name, generation, status string) account_api.Extension {
	var ext account_api.Extension
	ext.Name = name
	ext.Generation.Name = generation
	ext.Generation.Description = generation
	ext.Status.Name = status
	ext.Producer.Name = "Acme"
	ext.Producer.Id = 1
	return ext
}

func TestFilterProducerExtensions(t *testing.T) {
	extensions := []account_api.Extension{
		testExtension("PaymentPlugin", "platform", "approved"),
		testExtension("ShippingApp", "apps", "instore"),
		testExtension("Legacy", "classic", "approved"),
		testExtension("Gone", "platform", "deleted"),
		testExtension("", "platform", "approved"),
		testExtension("ThemePlugin", "platform", "incomplete"),
	}

	t.Run("no type filter keeps plugins and apps", func(t *testing.T) {
		got := filterProducerExtensions(extensions, false, false)
		assert.ElementsMatch(t, []string{"PaymentPlugin", "ShippingApp", "ThemePlugin"}, extensionNames(got))
	})

	t.Run("plugin filter", func(t *testing.T) {
		got := filterProducerExtensions(extensions, true, false)
		assert.ElementsMatch(t, []string{"PaymentPlugin", "ThemePlugin"}, extensionNames(got))
	})

	t.Run("app filter", func(t *testing.T) {
		got := filterProducerExtensions(extensions, false, true)
		assert.ElementsMatch(t, []string{"ShippingApp"}, extensionNames(got))
	})
}

func TestIncludeProducerExtension(t *testing.T) {
	assert.True(t, includeProducerExtension(testExtension("A", "platform", "approved"), false, false))
	assert.False(t, includeProducerExtension(testExtension("A", "classic", "approved"), false, false))
	assert.False(t, includeProducerExtension(testExtension("A", "platform", "deleted"), false, false))
	assert.False(t, includeProducerExtension(testExtension("", "platform", "approved"), false, false))
	assert.False(t, includeProducerExtension(testExtension("A", "apps", "approved"), true, false))
	assert.True(t, includeProducerExtension(testExtension("A", "apps", "approved"), false, true))
	assert.True(t, includeProducerExtension(testExtension("A", "platform", "approved"), true, false))
}

func extensionNames(extensions []account_api.Extension) []string {
	names := make([]string, 0, len(extensions))
	for _, ext := range extensions {
		names = append(names, ext.Name)
	}
	return names
}

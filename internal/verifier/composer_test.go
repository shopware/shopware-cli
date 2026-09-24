package verifier

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestShopwarePlatformPinsOnlyRequiredPlatformPackages(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), `{
		"require": {
			"shopware/core": "~6.6.0",
			"shopware/storefront": "~6.6.0",
			"guzzlehttp/guzzle": "^7.0"
		}
	}`)

	assert.Equal(t, []string{"shopware/core:6.7.14.2", "shopware/storefront:6.7.14.2"}, shopwarePlatformPins(dir, "6.7.14.2"))
	assert.Nil(t, shopwarePlatformPins(dir, ""))
	assert.Nil(t, shopwarePlatformPins(t.TempDir(), "6.7.14.2"))
}

func TestInstalledShopwareVersion(t *testing.T) {
	t.Parallel()

	composer2 := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(composer2, "vendor", "composer", "installed.json"), `{
		"packages": [
			{"name": "symfony/console", "version": "v7.3.0"},
			{"name": "shopware/core", "version": "v6.6.10.21"}
		]
	}`)
	assert.Equal(t, "6.6.10.21", installedShopwareVersion(composer2))

	composer1 := t.TempDir()
	testhelper.WriteFile(t, filepath.Join(composer1, "vendor", "composer", "installed.json"), `[
		{"name": "shopware/core", "version": "6.5.8.18"}
	]`)
	assert.Equal(t, "6.5.8.18", installedShopwareVersion(composer1))

	assert.Empty(t, installedShopwareVersion(t.TempDir()))
}

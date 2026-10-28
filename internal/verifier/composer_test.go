package verifier

import (
	"encoding/json"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidationComposerJSONIgnoresPackageIdentity(t *testing.T) {
	first, err := validationComposerJSON([]byte(`{
		"name": "acme/first",
		"type": "shopware-platform-plugin",
		"autoload": {"psr-4": {"Acme\\First\\": "src/"}},
		"extra": {"shopware-plugin-class": "Acme\\First\\First"},
		"require": {"shopware/core": "~6.6.0"}
	}`))
	require.NoError(t, err)

	second, err := validationComposerJSON([]byte(`{
		"name": "acme/second",
		"version": "2.0.0",
		"autoload": {"psr-4": {"Acme\\Second\\": "src/"}},
		"require": {"shopware/core": "~6.6.0"}
	}`))
	require.NoError(t, err)

	assert.Equal(t, string(first), string(second))
}

func TestValidationComposerJSONRequiresSuggestedPackages(t *testing.T) {
	result, err := validationComposerJSON([]byte(`{
		"require": {"shopware/core": "~6.6.0", "psr/log": "^3.0"},
		"suggest": {"psr/log": "already required", "psr/container": "optional"}
	}`))
	require.NoError(t, err)

	var data struct {
		Require map[string]string `json:"require"`
	}
	require.NoError(t, json.Unmarshal(result, &data))

	assert.Equal(t, map[string]string{
		"shopware/core": "~6.6.0",
		"psr/log":       "^3.0",
		"psr/container": "*",
	}, data.Require)
}

func TestValidationComposerJSONKeepsResolutionSettings(t *testing.T) {
	result, err := validationComposerJSON([]byte(`{
		"require": {"shopware/core": "*"},
		"repositories": [{"type": "composer", "url": "https://packages.example.com"}],
		"minimum-stability": "dev",
		"prefer-stable": true
	}`))
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(result, &data))

	assert.ElementsMatch(t, []string{"require", "repositories", "minimum-stability", "prefer-stable"}, slices.Collect(maps.Keys(data)))
}

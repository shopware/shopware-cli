package esbuild

import (
	"testing"

	"github.com/bep/godartsass/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScssImporterCanonicalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{name: "variables", url: "~scss/variables", expected: InternalVariablesScssPath},
		{name: "variables with extension", url: "~scss/variables.scss", expected: InternalVariablesScssPath},
		{name: "mixins", url: "~scss/mixins", expected: InternalMixinsScssPath},
		{name: "mixins with extension", url: "~scss/mixins.scss", expected: InternalMixinsScssPath},
		{name: "unknown import", url: "./custom.scss", expected: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			canonicalURL, err := (scssImporter{}).CanonicalizeURL(test.url)

			require.NoError(t, err)
			assert.Equal(t, test.expected, canonicalURL)
		})
	}
}

func TestScssImporterLoad(t *testing.T) {
	tests := []struct {
		name             string
		canonicalURL     string
		expectedContents string
	}{
		{name: "variables", canonicalURL: InternalVariablesScssPath, expectedContents: string(scssVariables)},
		{name: "mixins", canonicalURL: InternalMixinsScssPath, expectedContents: string(scssMixins)},
		{name: "unknown import", canonicalURL: "file://custom.scss", expectedContents: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := (scssImporter{}).Load(test.canonicalURL)

			require.NoError(t, err)
			assert.Equal(t, test.expectedContents, result.Content)
			assert.Equal(t, godartsass.SourceSyntaxSCSS, result.SourceSyntax)
		})
	}
}

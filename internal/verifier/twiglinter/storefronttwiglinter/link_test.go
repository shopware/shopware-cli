package storefronttwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestLinkDetection(t *testing.T) {
	cases := []struct {
		name          string
		content       string
		expectedCount int
	}{
		{
			name:          "Valid external link",
			content:       `<a href="https://google.com" target="_blank" rel="noopener">Test</a>`,
			expectedCount: 0,
		},
		{
			name:          "External link without rel",
			content:       `<a href="https://google.com" target="_blank">Test</a>`,
			expectedCount: 1,
		},
		{
			name:          "External link without target",
			content:       `<a href="https://google.com">Test</a>`,
			expectedCount: 2, // missing target and missing rel
		},
		{
			name:          "Internal link",
			content:       `<a href="/internal">Test</a>`,
			expectedCount: 0,
		},
		{
			name:          "Valid external link with noopener and noreferrer",
			content:       `<a href="https://google.com" target="_blank" rel="noopener noreferrer">Test</a>`,
			expectedCount: 0,
		},
		{
			name:          "External link with rel but not noopener",
			content:       `<a href="https://google.com" target="_blank" rel="noreferrer">Test</a>`,
			expectedCount: 1,
		},
		{
			name:          "Empty content",
			content:       ``,
			expectedCount: 0,
		},
		{
			name:          "A tag without href",
			content:       `<a>Test</a>`,
			expectedCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checks, err := twiglinter.RunCheckerOnString(LinkCheck{}, tc.content)
			assert.NoError(t, err)
			assert.Len(t, checks, tc.expectedCount)
		})
	}
}

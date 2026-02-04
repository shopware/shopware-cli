package storefronttwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestStyleDetection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "Inline style",
			content:  `<div style="color: red;">Test</div>`,
			expected: true,
		},
		{
			name:     "No inline style",
			content:  `<div class="test">Test</div>`,
			expected: false,
		},
		{
			name:     "Empty content",
			content:  ``,
			expected: false,
		},
		{
			name:     "Multiple styles",
			content:  `<div style="color: red; background: blue;">Test</div>`,
			expected: true,
		},
		{
			name:     "Style in style tag",
			content:  `<style>div { color: red; }</style><div>Test</div>`,
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checks, err := twiglinter.RunCheckerOnString(StyleFixer{}, tc.content)
			assert.NoError(t, err)
			if tc.expected {
				assert.NotEmpty(t, checks)
			} else {
				assert.Empty(t, checks)
			}
		})
	}
}

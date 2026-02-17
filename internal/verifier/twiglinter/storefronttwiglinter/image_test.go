package storefronttwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestImageAltDetection(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		content       string
		expectedCount int
	}{
		{
			name:          "Valid image with meaningful alt text",
			content:       `<img src="test.jpg" alt="A beautiful sunset over the mountains">`,
			expectedCount: 0,
		},
		{
			name:          "Image without alt attribute",
			content:       `<img src="test.jpg">`,
			expectedCount: 1,
		},
		{
			name:          "Image with empty alt attribute",
			content:       `<img src="test.jpg" alt="">`,
			expectedCount: 1, // Info level warning for empty alt
		},
		{
			name:          "Image with whitespace-only alt attribute",
			content:       `<img src="test.jpg" alt="   ">`,
			expectedCount: 1,
		},
		{
			name: "Multiple images - some valid, some invalid",
			content: `
				<img src="valid.jpg" alt="Valid description">
				<img src="invalid1.jpg">
				<img src="invalid2.jpg" alt="">
				<img src="valid2.jpg" alt="Another valid description">
			`,
			expectedCount: 2, // Two invalid images
		},
		{
			name:          "Image with other attributes and valid alt",
			content:       `<img src="test.jpg" class="responsive" width="100" height="100" alt="Test image description">`,
			expectedCount: 0,
		},
		{
			name:          "Image with Twig variables in alt",
			content:       `<img src="{{ image.url }}" alt="{{ image.description }}">`,
			expectedCount: 0,
		},
		{
			name:          "Image with Twig variables but no alt",
			content:       `<img src="{{ image.url }}" class="{{ image.class }}">`,
			expectedCount: 1,
		},
		{
			name: "Nested image in complex HTML",
			content: `
				<div class="gallery">
					<figure>
						<img src="gallery1.jpg" alt="First gallery image">
					</figure>
					<figure>
						<img src="gallery2.jpg">
					</figure>
				</div>
			`,
			expectedCount: 1, // Second image missing alt
		},
		{
			name:          "Self-closing image tag",
			content:       `<img src="test.jpg" alt="Self-closing tag" />`,
			expectedCount: 0,
		},
		{
			name:          "Self-closing image tag without alt",
			content:       `<img src="test.jpg" />`,
			expectedCount: 1,
		},
		{
			name:          "Empty content",
			content:       ``,
			expectedCount: 0,
		},
		{
			name:          "No image tags",
			content:       `<div><p>Some text without images</p></div>`,
			expectedCount: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checks, err := twiglinter.RunCheckerOnString(ImageAltCheck{}, tc.content)
			assert.NoError(t, err)
			assert.Len(t, checks, tc.expectedCount, "Expected %d validation errors but got %d", tc.expectedCount, len(checks))
		})
	}
}

func TestImageAltCheckIdentifiers(t *testing.T) {
	t.Parallel()
	// Test that correct identifiers are used for different types of errors
	missingAltChecks, err := twiglinter.RunCheckerOnString(ImageAltCheck{}, `<img src="test.jpg">`)
	assert.NoError(t, err)
	assert.Len(t, missingAltChecks, 1)
	assert.Equal(t, "twig-linter/image-missing-alt", missingAltChecks[0].Identifier)

	emptyAltChecks, err := twiglinter.RunCheckerOnString(ImageAltCheck{}, `<img src="test.jpg" alt="">`)
	assert.NoError(t, err)
	assert.Len(t, emptyAltChecks, 1)
	assert.Equal(t, "twig-linter/image-empty-alt", emptyAltChecks[0].Identifier)
}

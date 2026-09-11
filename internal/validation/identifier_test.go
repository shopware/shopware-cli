package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentifierMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		result    string
		ignore    string
		wantMatch bool
	}{
		{
			name:      "exact match",
			result:    "metadata.description",
			ignore:    "metadata.description",
			wantMatch: true,
		},
		{
			name:      "child identifier",
			result:    "metadata.description.length.de-DE",
			ignore:    "metadata.description",
			wantMatch: true,
		},
		{
			name:      "nested prefix",
			result:    "metadata.description.length.de-DE",
			ignore:    "metadata.description.length",
			wantMatch: true,
		},
		{
			name:      "sibling field is not a match",
			result:    "metadata.description",
			ignore:    "metadata.name",
			wantMatch: false,
		},
		{
			name:      "partial field name is not a match",
			result:    "metadata.descriptionExtra",
			ignore:    "metadata.description",
			wantMatch: false,
		},
		{
			name:      "empty ignore never matches",
			result:    "metadata.description",
			ignore:    "",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantMatch, IdentifierMatches(tt.result, tt.ignore))
		})
	}
}

package extension

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStorefrontThemeDumpArgs(t *testing.T) {
	tests := []struct {
		name string
		opts StorefrontWatcherOptions
		want []string
	}{
		{
			name: "non-interactive without a selected theme",
			want: []string{"theme:dump", "--no-interaction"},
		},
		{
			name: "selected theme",
			opts: StorefrontWatcherOptions{ThemeID: "theme-id"},
			want: []string{"theme:dump", "theme-id"},
		},
		{
			name: "selected theme and domain",
			opts: StorefrontWatcherOptions{ThemeID: "theme-id", DomainURL: "https://example.com"},
			want: []string{"theme:dump", "theme-id", "https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, storefrontThemeDumpArgs(tt.opts))
		})
	}
}

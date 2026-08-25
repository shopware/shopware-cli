package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePath(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantPath  string
		wantQuery string
	}{
		{"leading api is not duplicated", "/api/search/product", "api/search/product", ""},
		{"bare path gets the api prefix", "search/product", "api/search/product", ""},
		{"underscore route", "/_info/version", "api/_info/version", ""},
		{"query string survives", "search/product?limit=5", "api/search/product", "limit=5"},
		// Documents a quirk: any path merely starting with the letters "api"
		// loses that prefix.
		{"api-prefixed word is mangled", "api-test", "api/-test", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := parsePath(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.wantPath, u.Path)
			assert.Equal(t, tc.wantQuery, u.RawQuery)
		})
	}
}

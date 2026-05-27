package packagist

import (
	"testing"

	"github.com/shyim/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConstraintsSatisfiedBy(t *testing.T) {
	t.Parallel()

	target, err := version.NewVersion("6.6.4.0")
	require.NoError(t, err)

	packages := []string{"shopware/core", "shopware/storefront"}

	tests := []struct {
		name     string
		requires map[string]string
		want     bool
	}{
		{"no requires", nil, true},
		{"unrelated package ignored", map[string]string{"symfony/console": "^6.0"}, true},
		{"satisfied", map[string]string{"shopware/core": "^6.6"}, true},
		{"not satisfied", map[string]string{"shopware/core": "~6.5.0"}, false},
		{"one of many not satisfied", map[string]string{"shopware/core": "^6.6", "shopware/storefront": "~6.5.0"}, false},
		{"unparseable treated as unsatisfied", map[string]string{"shopware/core": "not-a-constraint"}, false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ConstraintsSatisfiedBy(tt.requires, packages, target))
		})
	}
}

func TestBumpConstraint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"2.3.4", "^2.3.4"},
		{"1.0.0", "^1.0.0"},
		{"^2.0", "^2.0"},
		{"~1.2", "~1.2"},
		{">=3.0", ">=3.0"},
		{"1.0 | 2.0", "1.0 | 2.0"},
		{"", ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, BumpConstraint(tt.in), "BumpConstraint(%q)", tt.in)
	}
}

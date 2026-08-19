package shop

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/mysqldump"
)

func TestApplyLimitOverrides(t *testing.T) {
	cfg := &ConfigDump{}

	require.NoError(t, applyLimitOverrides(cfg, []string{"order=100", "product=5"}))

	assert.Equal(t, map[string]mysqldump.TableLimit{
		"order":   {Rows: 100},
		"product": {Rows: 5},
	}, cfg.Limit)
}

func TestApplyLimitOverridesKeepsConfiguredOrderBy(t *testing.T) {
	cfg := &ConfigDump{
		Limit: map[string]mysqldump.TableLimit{
			"order": {Rows: 10, OrderBy: "`order_number` DESC"},
		},
	}

	require.NoError(t, applyLimitOverrides(cfg, []string{"order=100"}))

	assert.Equal(t, mysqldump.TableLimit{Rows: 100, OrderBy: "`order_number` DESC"}, cfg.Limit["order"])
}

func TestApplyLimitOverridesValidation(t *testing.T) {
	cases := []struct {
		override    string
		expectedErr string
	}{
		{"order", `invalid --limit "order", expected format table=rows (e.g. order=100)`},
		{"order=abc", `invalid --limit "order=abc", rows must be a positive number`},
		{"order=0", `invalid --limit "order=0", rows must be a positive number`},
		{"order=-5", `invalid --limit "order=-5", rows must be a positive number`},
	}

	for _, tc := range cases {
		t.Run(tc.override, func(t *testing.T) {
			err := applyLimitOverrides(&ConfigDump{}, []string{tc.override})
			assert.EqualError(t, err, tc.expectedErr)
		})
	}
}

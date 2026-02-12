package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestLoaderFixer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		before string
		after  string
	}{
		{
			before: `<sw-loader />`,
			after:  `<mt-loader/>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(LoaderFixer{}, c.before)
		assert.NoError(t, err)
		assert.Equal(t, c.after, newStr)
	}
}

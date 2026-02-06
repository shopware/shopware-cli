package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestPopover(t *testing.T) {
	t.Parallel()
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			before: `<sw-popover></sw-popover>`,
			after:  `<mt-floating-ui :isOpened="true"></mt-floating-ui>`,
		},
		{
			before: `<sw-popover v-if="bla"></sw-popover>`,
			after:  `<mt-floating-ui :isOpened="bla"></mt-floating-ui>`,
		},
		{
			before: `<sw-popover :zIndex="123"></sw-popover>`,
			after:  `<mt-floating-ui :isOpened="true"></mt-floating-ui>`,
		},
		{
			before: `<sw-popover :resizeWidth="123"></sw-popover>`,
			after:  `<mt-floating-ui :isOpened="true"></mt-floating-ui>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(PopoverFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

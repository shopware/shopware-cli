package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestIconFixer(t *testing.T) {
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement with default size",
			before:      `<sw-icon name="regular-times-s"/>`,
			after: `<mt-icon
    name="regular-times-s"
    size="24px"
/>`,
		},
		{
			description: "replace small with size 16px",
			before:      `<sw-icon name="regular-times-s" small/>`,
			after: `<mt-icon
    name="regular-times-s"
    size="16px"
/>`,
		},
		{
			description: "replace large with size 32px",
			before:      `<sw-icon name="regular-times-s" large/>`,
			after: `<mt-icon
    name="regular-times-s"
    size="32px"
/>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(IconFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

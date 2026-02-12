package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestButtonFixer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-button>Save</sw-button>`,
			after:       `<mt-button>Save</mt-button>`,
		},
		{
			description: "remove variant ghost and add ghost attribute",
			before:      `<sw-button variant="ghost">Save</sw-button>`,
			after:       `<mt-button ghost>Save</mt-button>`,
		},
		{
			description: "replace danger variant with critical",
			before:      `<sw-button variant="danger">Delete</sw-button>`,
			after:       `<mt-button variant="critical">Delete</mt-button>`,
		},
		{
			description: "replace ghost-danger variant with critical and add ghost",
			before:      `<sw-button variant="ghost-danger">Delete</sw-button>`,
			after: `<mt-button
    variant="critical"
    ghost
>Delete</mt-button>`,
		},
		{
			description: "remove contrast variant",
			before:      `<sw-button variant="contrast">Info</sw-button>`,
			after:       `<mt-button>Info</mt-button>`,
		},
		{
			description: "remove context variant",
			before:      `<sw-button variant="context">Info</sw-button>`,
			after:       `<mt-button>Info</mt-button>`,
		},
		{
			description: "replace router-link with @click",
			before:      `<sw-button router-link="sw.example.route">Go to example</sw-button>`,
			after:       `<mt-button @click="this.$router.push('sw.example.route')">Go to example</mt-button>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(ButtonFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

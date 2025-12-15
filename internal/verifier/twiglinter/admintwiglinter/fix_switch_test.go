package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestSwitchFixer(t *testing.T) {
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-switch-field>Hello World</sw-switch-field>`,
			after:       `<mt-switch>Hello World</mt-switch>`,
		},
		{
			description: "replace v-model:value",
			before:      `<sw-switch-field v-model:value="foobar">Hello World</sw-switch-field>`,
			after:       `<mt-switch v-model="foobar">Hello World</mt-switch>`,
		},
		{
			description: "replace noMarginTop with removeTopMargin",
			before:      `<sw-switch-field noMarginTop />`,
			after:       `<mt-switch removeTopMargin/>`,
		},
		{
			description: "replace value with checked",
			before:      `<sw-switch-field value="true" />`,
			after:       `<mt-switch checked="true"/>`,
		},
		{
			description: "convert label slot to label prop",
			before: `<sw-switch-field><template #label>
        Foobar
    </template></sw-switch-field>`,
			after: `<mt-switch label="Foobar"></mt-switch>`,
		},
		{
			description: "remove hint slot and add comment node",
			before: `<sw-switch-field><template #hint>
        Foobar
    </template></sw-switch-field>`,
			after: `<mt-switch></mt-switch>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(SwitchFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

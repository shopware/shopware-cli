package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestPasswordFieldFixer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-password-field></sw-password-field>`,
			after:       `<mt-password-field></mt-password-field>`,
		},
		{
			description: "replace value with model-value",
			before:      `<sw-password-field value="Hello World"/>`,
			after:       `<mt-password-field model-value="Hello World"/>`,
		},
		{
			description: "replace v-model:value with v-model",
			before:      `<sw-password-field v-model:value="myValue"/>`,
			after:       `<mt-password-field v-model="myValue"/>`,
		},
		{
			description: "replace size medium with default",
			before:      `<sw-password-field size="medium"/>`,
			after:       `<mt-password-field size="default"/>`,
		},
		{
			description: "remove isInvalid attribute",
			before:      `<sw-password-field isInvalid/>`,
			after:       `<mt-password-field/>`,
		},
		{
			description: "replace update:value event",
			before:      `<sw-password-field @update:value="updateValue"/>`,
			after:       `<mt-password-field @update:model-value="updateValue"/>`,
		},
		{
			description: "remove base-field-mounted event",
			before:      `<sw-password-field @base-field-mounted="onFieldMounted"/>`,
			after:       `<mt-password-field/>`,
		},
		{
			description: "process label slot",
			before: `<sw-password-field>
    <template #label>
        My Label
    </template>
</sw-password-field>`,
			after: `<mt-password-field label="My label"></mt-password-field>`,
		},
		{
			description: "process hint slot",
			before: `<sw-password-field>
    <template #hint>
        My Hint
    </template>
</sw-password-field>`,
			after: `<mt-password-field hint="My hint"></mt-password-field>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(PasswordFieldFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

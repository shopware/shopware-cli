package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestEmailFieldFixer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-email-field></sw-email-field>`,
			after:       `<mt-email-field></mt-email-field>`,
		},
		{
			description: "replace value with model-value",
			before:      `<sw-email-field value="Hello World"/>`,
			after:       `<mt-email-field model-value="Hello World"/>`,
		},
		{
			description: "replace v-model:value with v-model",
			before:      `<sw-email-field v-model:value="myValue"/>`,
			after:       `<mt-email-field v-model="myValue"/>`,
		},
		{
			description: "replace size medium with default",
			before:      `<sw-email-field size="medium"/>`,
			after:       `<mt-email-field size="default"/>`,
		},
		{
			description: "remove isInvalid attribute",
			before:      `<sw-email-field isInvalid/>`,
			after:       `<mt-email-field/>`,
		},
		{
			description: "remove aiBadge attribute",
			before:      `<sw-email-field aiBadge/>`,
			after:       `<mt-email-field/>`,
		},
		{
			description: "replace update:value event",
			before:      `<sw-email-field @update:value="updateValue"/>`,
			after:       `<mt-email-field @update:model-value="updateValue"/>`,
		},
		{
			description: "remove base-field-mounted event",
			before:      `<sw-email-field @base-field-mounted="onFieldMounted"/>`,
			after:       `<mt-email-field/>`,
		},
		{
			description: "process label slot",
			before: `<sw-email-field>
    <template #label>
        My Label
    </template>
</sw-email-field>`,
			after: `<mt-email-field label="My Label"></mt-email-field>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(EmailFieldFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

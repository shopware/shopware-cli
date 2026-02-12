package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestTextFieldFixer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-text-field></sw-text-field>`,
			after:       `<mt-text-field></mt-text-field>`,
		},
		{
			description: "replace value with model-value",
			before:      `<sw-text-field value="Hello World"/>`,
			after:       `<mt-text-field model-value="Hello World"/>`,
		},
		{
			description: "replace v-model:value with v-model",
			before:      `<sw-text-field v-model:value="myValue"/>`,
			after:       `<mt-text-field v-model="myValue"/>`,
		},
		{
			description: "convert size medium to default",
			before:      `<sw-text-field size="medium"/>`,
			after:       `<mt-text-field size="default"/>`,
		},
		{
			description: "remove isInvalid prop",
			before:      `<sw-text-field isInvalid />`,
			after:       `<mt-text-field/>`,
		},
		{
			description: "remove aiBadge prop",
			before:      `<sw-text-field aiBadge />`,
			after:       `<mt-text-field/>`,
		},
		{
			description: "replace update:value event",
			before:      `<sw-text-field @update:value="updateValue"/>`,
			after:       `<mt-text-field @update:model-value="updateValue"/>`,
		},
		{
			description: "remove base-field-mounted event",
			before:      `<sw-text-field @base-field-mounted="onFieldMounted" />`,
			after:       `<mt-text-field/>`,
		},
		{
			description: "process label slot conversion",
			before:      `<sw-text-field><template #label>My Label</template></sw-text-field>`,
			after:       `<mt-text-field label="My Label"></mt-text-field>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(TextFieldFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

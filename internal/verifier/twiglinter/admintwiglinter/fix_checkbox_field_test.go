package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestCheckboxFieldFixer(t *testing.T) {
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-checkbox-field />`,
			after:       `<mt-checkbox/>`,
		},
		{
			description: "replace value with checked",
			before:      `<sw-checkbox-field :value="myValue"/>`,
			after:       `<mt-checkbox :checked="myValue"/>`,
		},
		{
			description: "replace v-model with v-model:checked",
			before:      `<sw-checkbox-field v-model="isCheckedValue"/>`,
			after:       `<mt-checkbox v-model:checked="isCheckedValue"/>`,
		},
		{
			description: "replace v-model:value with v-model:checked",
			before:      `<sw-checkbox-field v-model:value="isCheckedValue"/>`,
			after:       `<mt-checkbox v-model:checked="isCheckedValue"/>`,
		},
		{
			description: "convert label slot to label prop",
			before: `<sw-checkbox-field><template #label>
        Hello Shopware
    </template></sw-checkbox-field>`,
			after: `<mt-checkbox label="Hello Shopware"></mt-checkbox>`,
		},
		{
			description: "remove hint slot",
			before: `<sw-checkbox-field><template v-slot:hint>
        Hello Shopware
    </template></sw-checkbox-field>`,
			after: `<mt-checkbox></mt-checkbox>`,
		},
		{
			description: "remove id and ghostValue props",
			before:      `<sw-checkbox-field id="checkbox-id" ghostValue="yes" />`,
			after:       `<mt-checkbox/>`,
		},
		{
			description: "convert partlyChecked to partial",
			before:      `<sw-checkbox-field partlyChecked/>`,
			after:       `<mt-checkbox partial/>`,
		},
		{
			description: "replace @update:value with @update:checked",
			before:      `<sw-checkbox-field @update:value="updateValue"/>`,
			after:       `<mt-checkbox @update:checked="updateValue"/>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(CheckboxFieldFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

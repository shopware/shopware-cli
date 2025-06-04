package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectFieldFixer(t *testing.T) {
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "replace value with model-value",
			before:      `<sw-select-field :value="selectedValue"/>`,
			after:       `<mt-select :model-value="selectedValue"/>`,
		},
		{
			description: "replace v-model:value with v-model",
			before:      `<sw-select-field v-model:value="selectedValue"/>`,
			after:       `<mt-select v-model="selectedValue"/>`,
		},
		{
			description: "convert options prop format",
			before:      `<sw-select-field :options="[ { label: 'Option 1', value: 1 }, { label: 'Option 2', value: 2 } ]"/>`,
			after: `<mt-select
    :options="[ { label: 'Option 1', value: 1 }, { label: 'Option 2', value: 2 } ]"
/>`,
		},
		{
			description: "convert default slot with option children to options prop",
			before: `<sw-select-field>
    <option value="1">Option 1</option>
    <option value="2">Option 2</option>
</sw-select-field>`,
			after: `<mt-select
    :options="[{&quot;label&quot;:&quot;Option 1&quot;,&quot;value&quot;:&quot;1&quot;},{&quot;label&quot;:&quot;Option 2&quot;,&quot;value&quot;:&quot;2&quot;}]"
></mt-select>`,
		},
		{
			description: "convert label slot to label prop",
			before:      `<sw-select-field><template #label>My Label</template></sw-select-field>`,
			after:       `<mt-select label="My Label"></mt-select>`,
		},
		{
			description: "replace update:value event with update:model-value",
			before:      `<sw-select-field @update:value="onUpdateValue"/>`,
			after:       `<mt-select @update:model-value="onUpdateValue"/>`,
		},
	}

	for _, c := range cases {
		newStr, err := runFixerOnString(SelectFieldFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

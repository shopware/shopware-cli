package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestColorpickerFixer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-colorpicker/>`,
			after:       `<mt-colorpicker/>`,
		},
		{
			description: "replace value with model-value",
			before:      `<sw-colorpicker :value="myValue"/>`,
			after:       `<mt-colorpicker :model-value="myValue"/>`,
		},
		{
			description: "replace v-model:value with v-model",
			before:      `<sw-colorpicker v-model:value="myValue"/>`,
			after:       `<mt-colorpicker v-model="myValue"/>`,
		},
		{
			description: "replace update:value event",
			before:      `<sw-colorpicker @update:value="onUpdateValue"/>`,
			after:       `<mt-colorpicker @update:model-value="onUpdateValue"/>`,
		},
		{
			description: "process label slot",
			before:      `<sw-colorpicker><template #label>My Label</template></sw-colorpicker>`,
			after:       `<mt-colorpicker label="My Label"></mt-colorpicker>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(ColorpickerFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

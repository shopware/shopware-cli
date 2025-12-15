package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestNumberFieldFixer(t *testing.T) {
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-number-field />`,
			after:       `<mt-number-field/>`,
		},
		{
			description: "replace :value with :model-value",
			before:      `<sw-number-field :value="5"/>`,
			after:       `<mt-number-field :model-value="5"/>`,
		},
		{
			description: "convert v-model:value to :model-value and @change",
			before:      `<sw-number-field v-model:value="myValue"/>`,
			after:       `<mt-number-field v-model="myValue"/>`,
		},
		{
			description: "convert label slot to label prop",
			before:      `<sw-number-field><template #label>My Label</template></sw-number-field>`,
			after:       `<mt-number-field label="My Label"></mt-number-field>`,
		},
		{
			description: "replace @update:value with @change",
			before:      `<sw-number-field @update:value="updateValue"/>`,
			after:       `<mt-number-field @change="updateValue"/>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(NumberFieldFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

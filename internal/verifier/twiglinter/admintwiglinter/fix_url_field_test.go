package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestUrlFieldFixer(t *testing.T) {
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "replace value with model-value",
			before:      `<sw-url-field value="Hello World"/>`,
			after:       `<mt-url-field model-value="Hello World"/>`,
		},
		{
			description: "replace v-model:value with v-model",
			before:      `<sw-url-field v-model:value="myValue"/>`,
			after:       `<mt-url-field v-model="myValue"/>`,
		},
		{
			description: "replace update:value event",
			before:      `<sw-url-field @update:value="updateValue"/>`,
			after:       `<mt-url-field @update:model-value="updateValue"/>`,
		},
		{
			description: "process label slot",
			before:      `<sw-url-field><template #label>My Label</template></sw-url-field>`,
			after:       `<mt-url-field label="My Label"></mt-url-field>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(UrlFieldFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

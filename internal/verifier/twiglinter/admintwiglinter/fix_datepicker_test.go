package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestDatepickerFixer(t *testing.T) {
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-datepicker :model-value="myValue" v-model="myValue" @update:model-value="handler"/>`,
			after: `<mt-datepicker
    :model-value="myValue"
    v-model="myValue"
    @update:model-value="handler"
/>`,
		},
		{
			description: "convert label slot to prop",
			before:      `<sw-datepicker><template #label>My Label</template></sw-datepicker>`,
			after:       `<mt-datepicker label="My Label"></mt-datepicker>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(DatepickerFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

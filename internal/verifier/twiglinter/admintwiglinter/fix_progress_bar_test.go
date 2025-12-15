package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestProgressBarFixer(t *testing.T) {
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "replace value with model-value",
			before:      `<sw-progress-bar value="5"/>`,
			after:       `<mt-progress-bar model-value="5"/>`,
		},
		{
			description: "replace v-model:value with v-model",
			before:      `<sw-progress-bar v-model:value="myValue"/>`,
			after:       `<mt-progress-bar v-model="myValue"/>`,
		},
		{
			description: "replace update:value event",
			before:      `<sw-progress-bar @update:value="updateValue"/>`,
			after:       `<mt-progress-bar @update:model-value="updateValue"/>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(ProgressBarFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestTextarea(t *testing.T) {
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-textarea-field><template #label>FOO</template></sw-textarea-field>`,
			after:       `<mt-textarea label="FOO"></mt-textarea>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(TextareaFieldFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

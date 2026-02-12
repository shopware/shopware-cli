package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestCardFixer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-card>Hello World</sw-card>`,
			after:       `<mt-card>Hello World</mt-card>`,
		},
		{
			description: "remove contentPadding property",
			before:      `<sw-card contentPadding="true">Hello World</sw-card>`,
			after:       `<mt-card>Hello World</mt-card>`,
		},
		{
			description: "convert aiBadge property to title slot",
			before:      `<sw-card aiBadge>Hello World</sw-card>`,
			after: `<mt-card>
    <slot name="title">
        <sw-ai-copilot-badge></sw-ai-copilot-badge>
    </slot>
    Hello World
</mt-card>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(CardFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

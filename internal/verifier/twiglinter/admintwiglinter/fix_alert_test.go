package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestAlertFixer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "basic component replacement",
			before:      `<sw-alert>Message</sw-alert>`,
			after:       `<mt-banner>Message</mt-banner>`,
		},
		{
			description: "info variant remains unchanged",
			before:      `<sw-alert variant="info">Info message</sw-alert>`,
			after:       `<mt-banner variant="info">Info message</mt-banner>`,
		},
		{
			description: "success variant converts to positive",
			before:      `<sw-alert variant="success">Success message</sw-alert>`,
			after:       `<mt-banner variant="positive">Success message</mt-banner>`,
		},
		{
			description: "error variant converts to critical",
			before:      `<sw-alert variant="error">Error message</sw-alert>`,
			after:       `<mt-banner variant="critical">Error message</mt-banner>`,
		},
		{
			description: "warning variant converts to attention",
			before:      `<sw-alert variant="warning">Warning message</sw-alert>`,
			after:       `<mt-banner variant="attention">Warning message</mt-banner>`,
		},
		{
			description: "preserve other attributes",
			before:      `<sw-alert variant="error" title="Error Title" :closable="true">Error message</sw-alert>`,
			after: `<mt-banner
    variant="critical"
    title="Error Title"
    :closable="true"
>Error message</mt-banner>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(AlertFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

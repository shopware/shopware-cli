package admintwiglinter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestVueI18nTcFixer(t *testing.T) {
	t.Parallel()
	cases := []struct {
		description string
		before      string
		after       string
	}{
		{
			description: "replace $tc in text interpolation",
			before:      `<span>{{ $tc('foo.bar') }}</span>`,
			after:       `<span>{{ $t('foo.bar') }}</span>`,
		},
		{
			description: "replace $tc in bound attribute",
			before:      `<mt-button :label="$tc('foo.bar')"/>`,
			after:       `<mt-button :label="$t('foo.bar')"/>`,
		},
		{
			description: "replace $tc with pluralization arguments",
			before:      `<span>{{ $tc('foo.bar', count) }}</span>`,
			after:       `<span>{{ $t('foo.bar', count) }}</span>`,
		},
		{
			description: "replace $tc in event handler",
			before:      `<mt-button @click="show($tc('foo.bar'))"/>`,
			after:       `<mt-button @click="show($t('foo.bar'))"/>`,
		},
		{
			description: "replace this.$tc",
			before:      `<span>{{ this.$tc('foo.bar') }}</span>`,
			after:       `<span>{{ this.$t('foo.bar') }}</span>`,
		},
		{
			description: "replace multiple occurrences in one expression",
			before:      `<span>{{ $tc('a') + $tc('b') }}</span>`,
			after:       `<span>{{ $t('a') + $t('b') }}</span>`,
		},
		{
			description: "leave $t untouched",
			before:      `<span>{{ $t('foo.bar') }}</span>`,
			after:       `<span>{{ $t('foo.bar') }}</span>`,
		},
		{
			description: "do not touch identifiers that merely start with $tc",
			before:      `<span>{{ $tcustom('foo') }}</span>`,
			after:       `<span>{{ $tcustom('foo') }}</span>`,
		},
		{
			description: "do not touch static attribute values",
			before:      `<span title="$tc('foo')">x</span>`,
			after:       `<span title="$tc('foo')">x</span>`,
		},
	}

	for _, c := range cases {
		newStr, err := twiglinter.RunFixerOnString(VueI18nTcFixer{}, c.before)
		assert.NoError(t, err, c.description)
		assert.Equal(t, c.after, newStr, c.description)
	}
}

func TestVueI18nTcFixerCheck(t *testing.T) {
	t.Parallel()

	results, err := twiglinter.RunCheckerOnString(VueI18nTcFixer{}, `<mt-button :label="$tc('foo')">{{ $tc('bar') }}</mt-button>`)
	assert.NoError(t, err)
	assert.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, "vue-i18n-tc", r.Identifier)
		assert.Equal(t, validation.SeverityWarning, r.Severity)
	}
}

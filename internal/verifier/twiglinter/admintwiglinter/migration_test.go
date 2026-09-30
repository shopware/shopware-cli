package admintwiglinter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/html"
	"github.com/shopware/shopware-cli/internal/verifier/twiglinter"
)

func TestMigrationBindings(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name          string
		fixer         twiglinter.TwigFixer
		before, after string
	}{
		{"progress binding", ProgressBarFixer{}, `<sw-progress-bar :value="progress"/>`, `<mt-progress-bar :model-value="progress"/>`},
		{"one way text expression", TextFieldFixer{}, `<sw-text-field :value="first + last"/>`, `<mt-text-field :model-value="first + last"/>`},
		{"switch value", SwitchFixer{}, `<sw-switch-field :value="enabled" @update:value="setEnabled"/>`, `<mt-switch :model-value="enabled" @update:model-value="setEnabled"/>`},
		{"textarea events", TextareaFieldFixer{}, `<sw-textarea-field :value="text" @update:value="setText"/>`, `<mt-textarea :model-value="text" @update:model-value="setText"/>`},
		{"email binding", EmailFieldFixer{}, `<sw-email-field :value="email"/>`, `<mt-email-field :model-value="email"/>`},
		{"password binding", PasswordFieldFixer{}, `<sw-password-field :value="password"/>`, `<mt-password-field :model-value="password"/>`},
		{"url binding", UrlFieldFixer{}, `<sw-url-field :value="url"/>`, `<mt-url-field :model-value="url"/>`},
		{"number update", NumberFieldFixer{}, `<sw-number-field @update:value="setNumber"/>`, `<mt-number-field @update:model-value="setNumber"/>`},
		{"translated label", TextFieldFixer{}, `<sw-text-field><template #label>{{ $tc('Label') }}</template></sw-text-field>`, `<mt-text-field :label="$tc('Label')"></mt-text-field>`},
		{"long form label", EmailFieldFixer{}, `<sw-email-field><template v-slot:label>Email Label</template></sw-email-field>`, `<mt-email-field label="Email Label"></mt-email-field>`},
		{"password casing", PasswordFieldFixer{}, `<sw-password-field><template #hint>Hint Label</template></sw-password-field>`, `<mt-password-field hint="Hint Label"></mt-password-field>`},
		{"popover width", PopoverFixer{}, `<sw-popover :resize-width="matchWidth"/>`, `<mt-floating-ui :match-reference-width="matchWidth" :isOpened="true"/>`},
		{"bound icon size", IconFixer{}, `<sw-icon :size="iconSize"/>`, `<mt-icon :size="iconSize"/>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := twiglinter.RunFixerOnString(tc.fixer, tc.before)
			require.NoError(t, err)
			expected, err := html.NewParser(tc.after)
			require.NoError(t, err)
			assert.Equal(t, html.NodeList(expected).Dump(0), got)
			again, err := twiglinter.RunFixerOnString(tc.fixer, got)
			require.NoError(t, err)
			assert.Equal(t, got, again)
		})
	}
}

func TestManualMigrationPreservesTemplate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		fixer twiglinter.TwigFixer
		input string
	}{
		{"conditional card badge", CardFixer{}, `<sw-card :ai-badge="showBadge">Content</sw-card>`},
		{"label markup", TextFieldFixer{}, `<sw-text-field><template #label><strong>Label</strong></template></sw-text-field>`},
		{"mixed interpolation", PasswordFieldFixer{}, `<sw-password-field><template #label>Hello {{ name }}</template></sw-password-field>`},
		{"existing label", EmailFieldFixer{}, `<sw-email-field label="Original"><template #label>Other</template></sw-email-field>`},
		{"scoped label", UrlFieldFixer{}, `<sw-url-field><template #label="data">{{ data.label }}</template></sw-url-field>`},
		{"unsupported hint", ColorpickerFixer{}, `<sw-colorpicker><template #hint>Help</template></sw-colorpicker>`},
		{"option loop", SelectFieldFixer{}, `<sw-select-field><option v-for="item in items" :value="item.id">{{ item.name }}</option></sw-select-field>`},
		{"wrapped options", SelectFieldFixer{}, `<sw-select-field><template><option value="1">One</option></template></sw-select-field>`},
		{"option condition", SelectFieldFixer{}, `<sw-select-field><option v-if="visible" value="1">One</option></sw-select-field>`},
		{"option disabled", SelectFieldFixer{}, `<sw-select-field><option disabled value="1">One</option></sw-select-field>`},
		{"arbitrary options expression", SelectFieldFixer{}, `<sw-select-field :options="items.map(item => ({ name: item.name, id: item.id }))"/>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := twiglinter.RunFixerOnString(tc.fixer, tc.input)
			require.NoError(t, err)
			original, err := html.NewParser(tc.input)
			require.NoError(t, err)
			assert.Equal(t, html.NodeList(original).Dump(0), got)
			results, err := twiglinter.RunCheckerOnString(tc.fixer, got)
			require.NoError(t, err)
			assert.NotEmpty(t, results, "manual migration must remain visible to the checker")
		})
	}
}

func TestCardBadgeReusesTitleSlot(t *testing.T) {
	t.Parallel()
	got, err := twiglinter.RunFixerOnString(CardFixer{}, `<sw-card aiBadge><template #title>Title</template>Content</sw-card>`)
	require.NoError(t, err)
	assert.Contains(t, got, "Title")
	assert.Contains(t, got, "sw-ai-copilot-badge")
	assert.Equal(t, 1, strings.Count(got, "#title"))
	assert.NotContains(t, got, `<slot name="title">`)
}

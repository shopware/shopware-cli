package extension

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/verifier"
)

func TestExtensionValidationSelection(t *testing.T) {
	t.Run("default runs all checkers", func(t *testing.T) {
		tools, statuses, err := selectExtensionValidationTools("", "")
		require.NoError(t, err)
		assert.Len(t, tools, 8)
		assert.Empty(t, statuses)
		assert.True(t, slices.ContainsFunc(tools, requiresToolSetup))
	})

	for _, name := range []string{"prettier", "php-cs-fixer", "rector"} {
		t.Run("only "+name, func(t *testing.T) {
			tools, _, err := selectExtensionValidationTools(name, "")
			require.NoError(t, err)
			assert.Equal(t, []string{name}, toolNamesForValidation(tools))
			assert.True(t, slices.ContainsFunc(tools, requiresToolSetup))
		})
	}

	t.Run("only phpstan", func(t *testing.T) {
		tools, statuses, err := selectExtensionValidationTools("phpstan", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"phpstan"}, toolNamesForValidation(tools))
		assert.True(t, slices.ContainsFunc(tools, requiresToolSetup))
		assert.Contains(t, statuses, verifier.ToolResult{Name: "builtin", SkipReason: "not selected by --only"})
	})

	t.Run("Twig validation needs no external tools", func(t *testing.T) {
		tools, _, err := selectExtensionValidationTools("storefront-twig", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"storefront-twig"}, toolNamesForValidation(tools))
		assert.False(t, slices.ContainsFunc(tools, requiresToolSetup))
	})

	t.Run("default exclusion is reported", func(t *testing.T) {
		tools, statuses, err := selectExtensionValidationTools("", "rector")
		require.NoError(t, err)
		assert.NotContains(t, toolNamesForValidation(tools), "rector")
		assert.Contains(t, statuses, verifier.ToolResult{Name: "rector", SkipReason: "excluded by --exclude"})
	})

	t.Run("exclude applies after only", func(t *testing.T) {
		tools, statuses, err := selectExtensionValidationTools("phpstan,sw-cli", "sw-cli")
		require.NoError(t, err)
		assert.Equal(t, []string{"phpstan"}, toolNamesForValidation(tools))
		assert.Contains(t, statuses, verifier.ToolResult{Name: "builtin", SkipReason: "excluded by --exclude"})
	})

	t.Run("duplicate only values run once", func(t *testing.T) {
		tools, _, err := selectExtensionValidationTools("sw-cli,sw-cli", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"builtin"}, toolNamesForValidation(tools))
	})

	t.Run("unsupported operation lists checkers", func(t *testing.T) {
		_, _, err := selectExtensionValidationTools("symfony-xml", "")
		require.ErrorContains(t, err, `tool with name "symfony-xml" not found, possible tools:`)
		assert.NotContains(t, err.Error(), "symfony-xml,")
		assert.Contains(t, err.Error(), "phpstan")
	})

	t.Run("typo lists only checkers", func(t *testing.T) {
		_, _, err := selectExtensionValidationTools("phpsta", "")
		require.ErrorContains(t, err, `tool with name "phpsta" not found, possible tools:`)
		assert.Contains(t, err.Error(), "phpstan")
		assert.Contains(t, err.Error(), "prettier")
		assert.Contains(t, err.Error(), "rector")
	})

	t.Run("empty selection fails", func(t *testing.T) {
		_, _, err := selectExtensionValidationTools("sw-cli", "sw-cli")
		require.EqualError(t, err, "no validation checks selected after applying --exclude")
	})
}

func TestExtensionValidateFullFlagDeprecated(t *testing.T) {
	flag := extensionValidateCmd.PersistentFlags().Lookup("full")
	require.NotNil(t, flag)
	assert.NotEmpty(t, flag.Deprecated)
}

func TestExtensionFixAndFormatHaveExcludeFlag(t *testing.T) {
	assert.NotNil(t, extensionFixCmd.Flags().Lookup("exclude"))
	assert.NotNil(t, extensionFormat.Flags().Lookup("exclude"))
}

func toolNamesForValidation(tools verifier.ToolList[verifier.CheckTool]) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

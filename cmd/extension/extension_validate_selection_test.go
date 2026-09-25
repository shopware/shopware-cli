package extension

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

func toolStatusByName(t *testing.T, statuses []validation.ToolInvocationStatus, name string) validation.ToolInvocationStatus {
	t.Helper()
	for _, status := range statuses {
		if status.Name == name {
			return status
		}
	}
	t.Fatalf("missing tool status for %s", name)
	return validation.ToolInvocationStatus{}
}

func TestExtensionValidationSelection(t *testing.T) {
	t.Run("default runs all checkers", func(t *testing.T) {
		tools, statuses, err := selectExtensionValidationTools("", "")
		require.NoError(t, err)
		assert.Len(t, tools, 6)
		assert.Len(t, statuses, len(tools))
		assert.True(t, slices.ContainsFunc(tools, requiresToolSetup))
		assert.Equal(t, "invoked", toolStatusByName(t, statuses, "phpstan").Status)
	})

	t.Run("only phpstan", func(t *testing.T) {
		tools, statuses, err := selectExtensionValidationTools("phpstan", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"phpstan"}, toolNamesForValidation(tools))
		assert.True(t, slices.ContainsFunc(tools, requiresToolSetup))
		assert.Equal(t, "invoked", toolStatusByName(t, statuses, "phpstan").Status)
		assert.Equal(t, "not selected by --only", toolStatusByName(t, statuses, "sw-cli").Reason)
	})

	t.Run("Twig validation needs no external tools", func(t *testing.T) {
		tools, _, err := selectExtensionValidationTools("admin-twig", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"admin-twig"}, toolNamesForValidation(tools))
		assert.False(t, slices.ContainsFunc(tools, requiresToolSetup))
	})

	t.Run("default exclusion is reported", func(t *testing.T) {
		tools, statuses, err := selectExtensionValidationTools("", "phpstan")
		require.NoError(t, err)
		assert.NotContains(t, toolNamesForValidation(tools), "phpstan")
		assert.Equal(t, "excluded by --exclude", toolStatusByName(t, statuses, "phpstan").Reason)
	})

	t.Run("exclude applies after only", func(t *testing.T) {
		tools, statuses, err := selectExtensionValidationTools("phpstan,sw-cli", "sw-cli")
		require.NoError(t, err)
		assert.Equal(t, []string{"phpstan"}, toolNamesForValidation(tools))
		assert.Equal(t, "excluded by --exclude", toolStatusByName(t, statuses, "sw-cli").Reason)
	})

	t.Run("duplicate only values run once", func(t *testing.T) {
		tools, _, err := selectExtensionValidationTools("sw-cli,sw-cli", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"sw-cli"}, toolNamesForValidation(tools))
	})

	t.Run("unsupported operation lists checkers", func(t *testing.T) {
		_, _, err := selectExtensionValidationTools("prettier", "")
		require.ErrorContains(t, err, `tool with name "prettier" not found, possible tools:`)
		assert.NotContains(t, err.Error(), "prettier,")
		assert.Contains(t, err.Error(), "phpstan")
	})

	t.Run("typo lists only checkers", func(t *testing.T) {
		_, _, err := selectExtensionValidationTools("phpsta", "")
		require.ErrorContains(t, err, `tool with name "phpsta" not found, possible tools:`)
		assert.Contains(t, err.Error(), "phpstan")
		assert.NotContains(t, err.Error(), "prettier")
		assert.NotContains(t, err.Error(), "rector")
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

func toolNamesForValidation(tools verifier.ToolList[verifier.CheckTool]) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

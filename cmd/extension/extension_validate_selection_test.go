package extension

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/validation"
	"github.com/shopware/shopware-cli/internal/verifier"
)

func coverageByName(t *testing.T, checks []validation.CheckCoverage, name string) validation.CheckCoverage {
	t.Helper()
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("missing coverage for %s", name)
	return validation.CheckCoverage{}
}

func TestExtensionValidationSelection(t *testing.T) {
	t.Run("default runs only sw-cli", func(t *testing.T) {
		tools, checks, err := selectExtensionValidationTools(false, "", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"sw-cli"}, toolNamesForValidation(tools))
		assert.False(t, slices.ContainsFunc(tools, requiresToolSetup))
		assert.Equal(t, "not selected; use --full or --only", coverageByName(t, checks, "phpstan").Reason)
	})

	t.Run("only phpstan works without full", func(t *testing.T) {
		tools, checks, err := selectExtensionValidationTools(false, "phpstan", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"phpstan"}, toolNamesForValidation(tools))
		assert.True(t, slices.ContainsFunc(tools, requiresToolSetup))
		assert.Equal(t, "invoked", coverageByName(t, checks, "phpstan").Status)
		assert.Equal(t, "not selected by --only", coverageByName(t, checks, "sw-cli").Reason)
	})

	t.Run("Twig validation needs no external tools", func(t *testing.T) {
		tools, _, err := selectExtensionValidationTools(false, "admin-twig", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"admin-twig"}, toolNamesForValidation(tools))
		assert.False(t, slices.ContainsFunc(tools, requiresToolSetup))
	})

	t.Run("full selects all validation checks", func(t *testing.T) {
		tools, checks, err := selectExtensionValidationTools(true, "", "")
		require.NoError(t, err)
		assert.Len(t, tools, 6)
		assert.Len(t, checks, len(tools))
		assert.True(t, slices.ContainsFunc(tools, requiresToolSetup))
	})

	t.Run("exclude applies after only", func(t *testing.T) {
		tools, checks, err := selectExtensionValidationTools(false, "phpstan,sw-cli", "sw-cli")
		require.NoError(t, err)
		assert.Equal(t, []string{"phpstan"}, toolNamesForValidation(tools))
		assert.Equal(t, "excluded by --exclude", coverageByName(t, checks, "sw-cli").Reason)
	})

	t.Run("duplicate only values run once", func(t *testing.T) {
		tools, _, err := selectExtensionValidationTools(false, "sw-cli,sw-cli", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"sw-cli"}, toolNamesForValidation(tools))
	})

	t.Run("unsupported operation fails", func(t *testing.T) {
		_, _, err := selectExtensionValidationTools(false, "prettier", "")
		require.EqualError(t, err, "prettier does not provide a validation check")
	})

	t.Run("empty selection fails", func(t *testing.T) {
		_, _, err := selectExtensionValidationTools(false, "sw-cli", "sw-cli")
		require.EqualError(t, err, "no validation checks selected after applying --exclude")
	})
}

func toolNamesForValidation(tools verifier.ToolList) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name())
	}
	return names
}

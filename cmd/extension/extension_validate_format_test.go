package extension

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtensionValidateFormatFlags(t *testing.T) {
	format := extensionValidateCmd.Flags().Lookup("format")
	require.NotNil(t, format)
	assert.False(t, format.Hidden)
	assert.Empty(t, format.Deprecated)

	reporter := extensionValidateCmd.Flags().Lookup("reporter")
	require.NotNil(t, reporter)
	assert.True(t, reporter.Hidden)
	assert.NotEmpty(t, reporter.Deprecated)

	format.Changed = true
	reporter.Changed = true
	t.Cleanup(func() {
		format.Changed = false
		reporter.Changed = false
	})
	assert.Error(t, extensionValidateCmd.ValidateFlagGroups())
}

func TestExtensionValidationFormatSupportsNewAndDeprecatedFlags(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")

	newFlag := newExtensionValidationFormatCommand()
	require.NoError(t, newFlag.Flags().Set("format", "markdown"))
	format, err := extensionValidationFormat(newFlag)
	require.NoError(t, err)
	assert.Equal(t, "markdown", format)

	deprecatedFlag := newExtensionValidationFormatCommand()
	require.NoError(t, deprecatedFlag.Flags().Set("reporter", "github"))
	format, err = extensionValidationFormat(deprecatedFlag)
	require.NoError(t, err)
	assert.Equal(t, "github", format)

	invalid := newExtensionValidationFormatCommand()
	require.NoError(t, invalid.Flags().Set("format", "yaml"))
	_, err = extensionValidationFormat(invalid)
	assert.Error(t, err)
}

func newExtensionValidationFormatCommand() *cobra.Command {
	command := &cobra.Command{}
	command.Flags().String("format", "", "")
	command.Flags().String("reporter", "", "")
	return command
}

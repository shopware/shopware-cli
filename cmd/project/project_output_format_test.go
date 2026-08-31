package project

import (
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	adminSdk "github.com/shopware/shopware-cli/internal/admin-api"
	"github.com/shopware/shopware-cli/internal/tui"
)

func TestProjectExtensionTablesPreserveJSONContract(t *testing.T) {
	extension := &adminSdk.ExtensionDetail{
		Name:          "Example",
		Version:       "1.0.0",
		LatestVersion: "1.1.0",
		Active:        true,
		UpdateSource:  "store",
	}
	extensions := adminSdk.ExtensionList{extension}

	expected, err := json.Marshal(extensions)
	require.NoError(t, err)

	for _, result := range []*tui.Table{
		projectExtensionListTable(extensions),
		projectExtensionOutdatedTable(extensions),
	} {
		output, err := result.Render(tui.TableFormatJSON)
		require.NoError(t, err)
		assert.JSONEq(t, string(expected), output)
	}
}

func TestProjectOutputFormatFlags(t *testing.T) {
	tests := []struct {
		name       string
		command    *cobra.Command
		legacyFlag string
		defaultVal string
	}{
		{
			name:       "extension list",
			command:    projectExtensionListCmd,
			legacyFlag: "json",
			defaultVal: "table",
		},
		{
			name:       "extension outdated",
			command:    projectExtensionOutdatedCmd,
			legacyFlag: "json",
			defaultVal: "table",
		},
		{
			name:       "validate",
			command:    projectValidateCmd,
			legacyFlag: "reporter",
			defaultVal: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			format := test.command.Flags().Lookup("format")
			require.NotNil(t, format)
			assert.False(t, format.Hidden)
			assert.Empty(t, format.Deprecated)
			assert.Equal(t, test.defaultVal, format.DefValue)

			legacy := test.command.Flags().Lookup(test.legacyFlag)
			require.NotNil(t, legacy)
			assert.True(t, legacy.Hidden)
			assert.NotEmpty(t, legacy.Deprecated)

			format.Changed = true
			legacy.Changed = true
			t.Cleanup(func() {
				format.Changed = false
				legacy.Changed = false
			})
			assert.Error(t, test.command.ValidateFlagGroups())
		})
	}
}

func TestProjectValidationFormatSupportsNewAndDeprecatedFlags(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("GITLAB_CI", "")

	newFlag := newValidationFormatCommand()
	require.NoError(t, newFlag.Flags().Set("format", "json"))
	format, err := projectValidationFormat(newFlag)
	require.NoError(t, err)
	assert.Equal(t, "json", format)

	deprecatedFlag := newValidationFormatCommand()
	require.NoError(t, deprecatedFlag.Flags().Set("reporter", "junit"))
	format, err = projectValidationFormat(deprecatedFlag)
	require.NoError(t, err)
	assert.Equal(t, "junit", format)

	defaultFormat, err := projectValidationFormat(newValidationFormatCommand())
	require.NoError(t, err)
	assert.Equal(t, "summary", defaultFormat)
}

func TestProjectExtensionDeprecatedJSONAlias(t *testing.T) {
	format, err := projectExtensionOutputFormat("table", true)
	require.NoError(t, err)
	assert.Equal(t, tui.TableFormatJSON, format)
}

func newValidationFormatCommand() *cobra.Command {
	command := &cobra.Command{}
	command.Flags().String("format", "", "")
	command.Flags().String("reporter", "", "")
	return command
}

package project

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/system"
)

func TestOnlyExtensionsAcceptsSpaceSeparatedValue(t *testing.T) {
	t.Parallel()

	var onlyExtensions string
	command := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, args []string) error {
			onlyExtensions, _ = cmd.Flags().GetString("only-extensions")
			return nil
		},
	}
	command.Flags().String("only-extensions", "", "extensions")
	command.SetArgs([]string{"--only-extensions", "PluginA,PluginB"})

	require.NoError(t, command.Execute())
	assert.Equal(t, "PluginA,PluginB", onlyExtensions)
}

func TestSelectExtensionsRequiresInteractiveTerminal(t *testing.T) {
	t.Parallel()

	err := validateExtensionSelection(system.WithInteraction(context.Background(), false), "", true)
	require.EqualError(t, err, "--select-extensions requires an interactive terminal; use --only-extensions with a comma-separated list instead")
}

func TestSelectExtensionsCannotBeCombinedWithOnlyExtensions(t *testing.T) {
	t.Parallel()

	err := validateExtensionSelection(context.Background(), "PluginA", true)
	require.EqualError(t, err, "only one of --only-extensions and --select-extensions can be used")
}

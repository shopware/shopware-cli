package ai

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveFormat(t *testing.T) {
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		addFormatFlag(cmd)

		return cmd
	}

	t.Run("defaults to table", func(t *testing.T) {
		format, err := resolveFormat(newCmd())
		require.NoError(t, err)
		assert.Equal(t, formatTable, format)
	})

	t.Run("explicit json", func(t *testing.T) {
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("format", "json"))

		format, err := resolveFormat(cmd)
		require.NoError(t, err)
		assert.Equal(t, formatJSON, format)
	})

	t.Run("invalid", func(t *testing.T) {
		cmd := newCmd()
		require.NoError(t, cmd.Flags().Set("format", "xml"))

		_, err := resolveFormat(cmd)
		assert.ErrorContains(t, err, "unknown --format")
	})
}

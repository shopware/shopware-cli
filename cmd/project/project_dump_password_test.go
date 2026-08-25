package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/system"
)

// The prompt-sentinel guards keep dump from hanging in CI waiting for a
// password nobody can type.
func TestAssembleConnectionURIPasswordPromptErrors(t *testing.T) {
	chdirOutsideProject(t)
	t.Setenv("DATABASE_URL", "")

	t.Run("interaction disabled", func(t *testing.T) {
		cmd := newDumpFlagCommand(t, nil)
		require.NoError(t, cmd.Flags().Set("password", passwordFlagPrompt))
		cmd.SetContext(system.WithInteraction(t.Context(), false))

		_, err := assembleConnectionURI(cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot prompt for password: interaction disabled")
	})

	t.Run("stdin is not a terminal", func(t *testing.T) {
		cmd := newDumpFlagCommand(t, nil)
		require.NoError(t, cmd.Flags().Set("password", passwordFlagPrompt))

		_, err := assembleConnectionURI(cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cannot prompt for password: stdin is not a terminal")
	})
}

package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectComposerCommandRegistered(t *testing.T) {
	cmd, _, err := projectRootCmd.Find([]string{"composer"})
	require.NoError(t, err)
	assert.Equal(t, "composer", cmd.Name())
	assert.True(t, cmd.DisableFlagParsing)
}

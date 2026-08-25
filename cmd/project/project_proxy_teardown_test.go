package project

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/proxy"
)

func TestConfirmTeardownRequiresForceWithoutTerminal(t *testing.T) {
	reg := proxy.Registry{Projects: []proxy.ProjectEntry{{ProjectRoot: t.TempDir(), Hostname: "shop.shopware.local"}}}

	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().Bool("force", false, "")
		cmd.SetContext(t.Context())
		return cmd
	}

	// Teardown affects every registered project; without a terminal it must
	// refuse unless --force is passed.
	cmd := newCmd()
	confirmed, err := confirmTeardown(cmd, reg)
	require.Error(t, err)
	assert.False(t, confirmed)
	assert.Contains(t, err.Error(), "--force")

	cmd = newCmd()
	require.NoError(t, cmd.Flags().Set("force", "true"))
	confirmed, err = confirmTeardown(cmd, reg)
	require.NoError(t, err)
	assert.True(t, confirmed)
}

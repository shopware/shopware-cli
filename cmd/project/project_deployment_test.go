//go:build deployment

package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeploymentCommandsRegistered(t *testing.T) {
	assert.Contains(t, projectRootCmd.Commands(), projectDeploymentCmd)
	assert.Contains(t, projectDeploymentCmd.Commands(), projectDeploymentPackageCmd)
	for _, name := range []string{"archive", "container"} {
		cmd, remaining, err := projectRootCmd.Find([]string{"deployment", "package", name})
		require.NoError(t, err)
		assert.Empty(t, remaining)
		assert.Equal(t, name, cmd.Name())
	}
}

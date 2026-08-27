package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unlike the extension commands, upgrade refuses to fall back to the current
// directory when no Shopware project is found.
func TestProjectUpgradeHeadlessRequiresProjectRoot(t *testing.T) {
	chdirOutsideProject(t)

	projectUpgradeCmd.SetContext(t.Context())
	err := projectUpgradeCmd.RunE(projectUpgradeCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot find Shopware project")
}

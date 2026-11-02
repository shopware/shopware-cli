package ssh

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSSHDeploymentLayout checks valid active-link layouts and reserved storage path rejection.
func TestSSHDeploymentLayout(t *testing.T) {
	for _, directory := range []string{"/srv/shop/current", "/srv/shop/live", "/var/www/shop", "/srv/shop/live/", "/srv/shop/live's shop"} {
		t.Run(directory, func(t *testing.T) {
			s := &SSH{directory: directory}
			root, err := s.deploymentRoot()
			require.NoError(t, err)
			assert.Equal(t, path.Dir(path.Clean(directory)), root)
			assert.Equal(t, path.Base(path.Clean(directory)), s.activeLinkName())
		})
	}
	for _, directory := range []string{"", ".", "relative/live", "/", "/shop", "/srv/shop/releases", "/srv/shop/shared", "/srv/shop/runtime", "/srv/shop/.shopware-cli", "/srv/shop/.current-event", "/srv/shop/live\x00", "/srv/shop/live\n", "/srv/shop\\live"} {
		t.Run(directory, func(t *testing.T) {
			_, err := (&SSH{directory: directory}).deploymentRoot()
			require.Error(t, err)
		})
	}
}

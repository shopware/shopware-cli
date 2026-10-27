package deployment

import (
	"encoding/json"
	"os/exec"
	"path"
	"strings"
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

// TestSSHRemoteLayoutRejectsUnsafeLinkNames checks that remote PHP enforces active-link path restrictions.
func TestSSHRemoteLayoutRejectsUnsafeLinkNames(t *testing.T) {
	php := deploymentPHP(t)
	root := t.TempDir()
	for _, name := range []string{"", ".", "..", "../outside", "/outside", "releases", "shared", "runtime", ".shopware-cli", ".current-event", "live\x00", "live\n", "live\\path"} {
		t.Run(name, func(t *testing.T) {
			input, err := json.Marshal(map[string]string{"current": name})
			require.NoError(t, err)
			script := strings.TrimPrefix(sshLayoutScript, "<?php\n") + `
try { echo deploymentCurrentPath(json_decode($argv[1], true), $argv[2]); }
catch (Throwable $error) { exit(1); }
`
			_, err = exec.CommandContext(t.Context(), php, "-r", script, "--", string(input), root).Output()
			require.Error(t, err)
		})
	}
}

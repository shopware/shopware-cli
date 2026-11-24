package ssh

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestSSHDeploymentLogsCancellationAndValidation(t *testing.T) {
	backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh",
		SSH:  &shop.EnvironmentSSHConfig{Host: "example.invalid", Directory: "/srv/shop/current"},
	})
	require.NoError(t, err)
	e := backend.(*SSH)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, e.WriteDeploymentLogs(ctx, Deployment{Reference: "happy-euclid"}, io.Discard), context.Canceled)
	t.Setenv("PATH", t.TempDir())
	for _, reference := range []string{"", "..", ".hidden", "name\ninjection"} {
		require.ErrorContains(t, e.WriteDeploymentLogs(t.Context(), Deployment{Reference: reference}, io.Discard), "invalid deployment name")
	}
}

func TestDeploymentLogReferences(t *testing.T) {
	root := t.TempDir()
	list := func() []string {
		data := runLocalDeploymentCommand(t, "logs", sshDeploymentLogsInput{Root: root, List: true})
		var references []string
		require.NoError(t, json.Unmarshal(data, &references))
		return references
	}
	assert.Empty(t, list())
	dir := filepath.Join(root, ".shopware-cli", "logs")
	for _, name := range []string{"successful.log", "failed.log", "ignore.txt", ".hidden.log"} {
		testhelper.WriteFile(t, filepath.Join(dir, name), "log")
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, "directory.log"), 0755))
	require.NoError(t, os.Symlink(filepath.Join(dir, "successful.log"), filepath.Join(dir, "linked.log")))
	assert.Equal(t, []string{"failed", "successful"}, list())
}

package ssh

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
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

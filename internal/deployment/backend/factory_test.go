package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

func TestNewBackend(t *testing.T) {
	for _, kind := range []string{"", "local", "docker", "unknown"} {
		t.Run(kind, func(t *testing.T) {
			_, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{Type: kind})
			require.ErrorIs(t, err, deployment.ErrNotSupported)
		})
	}
	_, err := New(t.TempDir(), "", &shop.Config{}, nil)
	require.ErrorIs(t, err, deployment.ErrNotSupported)
	for _, config := range []*shop.EnvironmentSSHConfig{
		nil, {}, {Host: "example.invalid"}, {Directory: "/srv/shop"},
	} {
		_, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{Type: "ssh", SSH: config})
		require.Error(t, err)
	}
	backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh", SSH: &shop.EnvironmentSSHConfig{Host: "example.invalid", Directory: "/srv/shop"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ssh", backend.Type())
	_, generalExecutor := backend.(executor.Executor)
	assert.False(t, generalExecutor, "deployment backends must not expose general execution")
}

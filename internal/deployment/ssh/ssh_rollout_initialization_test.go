package ssh

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/testhelper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRolloutRequiresInitialization(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "release.tar.gz")
	testhelper.WriteFile(t, archive, "archive")
	for _, inspectErr := range []error{nil, errors.New("inspection failed")} {
		store := &memoryInitializationStore{inspectErrs: [2]error{inspectErr}}
		backend := &SSH{name: "web", directory: "/var/www/shop/current", initStore: store}
		// No transport is supplied: reaching upload would panic.
		_, err := backend.RolloutDeployment(t.Context(), Deployment{Reference: archive}, nil)
		require.Error(t, err)
		if inspectErr != nil {
			require.ErrorIs(t, err, inspectErr)
		}
		assert.Equal(t, 1, store.inspections)
		assert.Empty(t, store.writes)
	}
}

func TestGroupRequiresEveryHostInitialized(t *testing.T) {
	ready := &memoryInitializationStore{states: [2]sshDeploymentInitState{{Initialized: true}}}
	missing := &memoryInitializationStore{}
	group := &sshGroup{hosts: []sshHost{
		{name: "a", backend: &SSH{name: "a", initStore: ready}},
		{name: "b", backend: &SSH{name: "b", initStore: missing}},
	}}
	require.Error(t, group.requireInitialized(t.Context()))
	assert.Equal(t, 1, ready.inspections)
	assert.Equal(t, 1, missing.inspections)
	assert.Empty(t, ready.writes)
	assert.Empty(t, missing.writes)
}

func TestInitializedHostAllowsRollout(t *testing.T) {
	store := &memoryInitializationStore{states: [2]sshDeploymentInitState{{Initialized: true}}}
	require.NoError(t, (&SSH{initStore: store}).requireInitialized(t.Context()))
}

func TestInitializationHostName(t *testing.T) {
	env := &shop.EnvironmentConfig{SSH: &shop.EnvironmentSSHConfig{Host: "shop.example.com"}}
	assert.Equal(t, "shop.example.com", (&SSH{env: env}).initializationHostName())
	assert.Equal(t, "web", (&SSH{name: "web", env: env}).initializationHostName())
	assert.NotEmpty(t, (&SSH{}).initializationHostName())
}

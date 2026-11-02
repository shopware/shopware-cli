package ssh

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/shop"
)

// fixtureInitializationGroup creates a two-host group with inspectable in-memory configuration stores.
func fixtureInitializationGroup(t *testing.T) (*sshGroup, map[string]*memoryInitializationStore) {
	t.Helper()
	backend, err := New(t.TempDir(), "", &shop.Config{}, &shop.EnvironmentConfig{
		Type: "ssh", SSH: &shop.EnvironmentSSHConfig{
			Directory: "/srv/shop/current", MigrationHost: "b", Parallelism: 2,
			// Initialization also works when environment variables are provided externally.
			Shared: &shop.EnvironmentSSHSharedConfig{Files: new([]string{})},
			Hosts:  map[string]*shop.EnvironmentSSHHostConfig{"a": {Host: "a.invalid"}, "b": {Host: "b.invalid"}},
		},
	})
	require.NoError(t, err)
	g := backend.(*sshGroup)
	stores := make(map[string]*memoryInitializationStore)
	for _, host := range g.hosts {
		state := sshDeploymentInitState{
			HasSharedDirectory: true, HasRuntimeConfig: true, RuntimeSHA256: "same",
			RuntimeKeys: []string{"APP_SECRET"},
		}
		store := &memoryInitializationStore{states: [2]sshDeploymentInitState{state, state}}
		host.backend.initStore = store
		stores[host.name] = store
	}
	return g, stores
}

// TestSSHGroupInitializationWritesOnceOnMigrationHost checks that only the migration host receives configuration.
func TestSSHGroupInitializationWritesOnceOnMigrationHost(t *testing.T) {
	g, stores := fixtureInitializationGroup(t)
	stores["a"].states[0] = sshDeploymentInitState{HasSharedDirectory: true, HasCurrent: true}
	config := sshDeploymentInitConfig{
		RuntimeValues: map[string]string{"APP_SECRET": "one-secret"},
		InstallValues: map[string]string{"INSTALL_ADMIN_PASSWORD": "one-password"},
	}
	calls := 0
	err := g.initializeDeployment(t.Context(), func(_ context.Context, state sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		calls++
		assert.True(t, state.HasCurrent, "current must be aggregated across hosts")
		assert.True(t, state.HasRuntimeConfig, "runtime state must come from migration host")
		assert.Contains(t, state.RuntimeKeys, "APP_SECRET")
		return config, true, nil
	})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)
	assert.Empty(t, stores["a"].writes)
	assert.Equal(t, []sshDeploymentInitConfig{config}, stores["b"].writes)
	for _, store := range stores {
		assert.Equal(t, 2, store.inspections)
	}
}

// TestSSHGroupInitializationPromptStopsWithoutWriting checks that cancellation and prompt errors leave every host untouched.
func TestSSHGroupInitializationPromptStopsWithoutWriting(t *testing.T) {
	promptErr := errors.New("prompt failed")
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{"canceled", nil, deployment.ErrInitializationCanceled},
		{"failed", promptErr, promptErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			g, stores := fixtureInitializationGroup(t)
			calls := 0
			err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
				calls++
				return sshDeploymentInitConfig{RuntimeValues: map[string]string{"APP_SECRET": "not-written"}}, false, test.err
			})
			require.ErrorIs(t, err, test.want)
			assert.Equal(t, 1, calls)
			for _, store := range stores {
				assert.Empty(t, store.writes)
				assert.Equal(t, 1, store.inspections)
			}
		})
	}
}

// TestSSHGroupInitializationApplyError checks that a failed migration-host write is returned to the caller.
func TestSSHGroupInitializationApplyError(t *testing.T) {
	g, stores := fixtureInitializationGroup(t)
	stores["b"].applyErr = errors.New("write failed")
	err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
		return sshDeploymentInitConfig{RuntimeValues: map[string]string{"APP_ENV": "prod"}}, true, nil
	})
	require.ErrorIs(t, err, stores["b"].applyErr)
	assert.ErrorContains(t, err, "host b:")
	assert.Empty(t, stores["a"].writes)
	assert.Len(t, stores["b"].writes, 1)
	for _, store := range stores {
		assert.Equal(t, 1, store.inspections)
	}
}

// TestSSHGroupInitializationRepeatVerifiesWithoutWriting checks that repeated initialization verifies shared state without another write.
func TestSSHGroupInitializationRepeatVerifiesWithoutWriting(t *testing.T) {
	for _, consistent := range []bool{true, false} {
		t.Run(fmt.Sprintf("consistent=%t", consistent), func(t *testing.T) {
			g, stores := fixtureInitializationGroup(t)
			stores["b"].states[0].Initialized = true
			if !consistent {
				stores["a"].states[1].RuntimeSHA256 = "different"
			}
			err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
				t.Fatal("completed initialization must not prompt")
				return sshDeploymentInitConfig{}, false, nil
			})
			if consistent {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "host a:")
			}
			for _, store := range stores {
				assert.Empty(t, store.writes)
				assert.Equal(t, 2, store.inspections)
			}
		})
	}
}

// TestSSHGroupInitializationPreflight checks that unsafe host states prevent prompting and writing.
func TestSSHGroupInitializationPreflight(t *testing.T) {
	unreachable := errors.New("host unreachable")
	for _, test := range []struct {
		name  string
		state sshDeploymentInitState
		err   error
	}{
		{"missing shared directory", sshDeploymentInitState{}, nil},
		{"stale install", sshDeploymentInitState{HasSharedDirectory: true, HasInstallConfig: true}, nil},
		{"unreachable", sshDeploymentInitState{}, unreachable},
	} {
		t.Run(test.name, func(t *testing.T) {
			g, stores := fixtureInitializationGroup(t)
			stores["a"].states[0], stores["a"].inspectErrs[0] = test.state, test.err
			err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
				t.Fatal("prompt must not run after failed preflight")
				return sshDeploymentInitConfig{}, false, nil
			})
			require.ErrorContains(t, err, "host a:")
			if test.err != nil {
				assert.ErrorIs(t, err, test.err)
			}
			for _, store := range stores {
				assert.Empty(t, store.writes)
				assert.Equal(t, 1, store.inspections)
			}
		})
	}
}

// TestSSHGroupInitializationVerification checks that missing or inconsistent shared configuration fails verification.
func TestSSHGroupInitializationVerification(t *testing.T) {
	unreachable := errors.New("host unreachable")
	for _, test := range []struct {
		name  string
		state sshDeploymentInitState
		err   error
	}{
		{"different", sshDeploymentInitState{HasRuntimeConfig: true, RuntimeSHA256: "different"}, nil},
		{"missing", sshDeploymentInitState{}, nil},
		{"missing hash", sshDeploymentInitState{HasRuntimeConfig: true}, nil},
		{"unreachable", sshDeploymentInitState{}, unreachable},
		{"stale install", sshDeploymentInitState{HasRuntimeConfig: true, RuntimeSHA256: "same", HasInstallConfig: true}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			g, stores := fixtureInitializationGroup(t)
			stores["a"].states[1], stores["a"].inspectErrs[1] = test.state, test.err
			err := g.initializeDeployment(t.Context(), func(context.Context, sshDeploymentInitState) (sshDeploymentInitConfig, bool, error) {
				return sshDeploymentInitConfig{RuntimeValues: map[string]string{"APP_ENV": "prod"}}, true, nil
			})
			require.ErrorContains(t, err, "host a:")
			if test.err != nil {
				assert.ErrorIs(t, err, test.err)
			}
			assert.Len(t, stores["b"].writes, 1, "verification does not roll back published config")
			assert.Empty(t, stores["a"].writes)
			for _, store := range stores {
				assert.Equal(t, 2, store.inspections)
			}
		})
	}
}

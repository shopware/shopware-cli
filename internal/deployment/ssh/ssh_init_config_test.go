package ssh

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/deployment"
	"github.com/shopware/shopware-cli/internal/envfile"
	"github.com/shopware/shopware-cli/internal/system"
)

func scriptedInitializationConfig() *deployment.InitializationConfig {
	return &deployment.InitializationConfig{RuntimeValues: map[string]string{
		"APP_URL": "https://shop.example.com", "DATABASE_URL": "mysql://user:secret@localhost/shop",
	}}
}

func TestSSHScriptedInitialization(t *testing.T) {
	for _, existingSecret := range []bool{true, false} {
		t.Run(map[bool]string{true: "preserve secret", false: "generate secret"}[existingSecret], func(t *testing.T) {
			store := &memoryInitializationStore{}
			if existingSecret {
				store.states[0].RuntimeKeys = []string{"APP_SECRET"}
			}
			backend := &SSH{initStore: store}
			config := scriptedInitializationConfig()
			require.NoError(t, backend.InitializeDeployment(t.Context(), deployment.InitializationOptions{Config: config}))
			require.Len(t, store.writes, 1)
			values := store.writes[0].RuntimeValues
			assert.Equal(t, "prod", values["APP_ENV"])
			assert.Equal(t, "0", values["APP_DEBUG"])
			if existingSecret {
				assert.NotContains(t, values, "APP_SECRET")
			} else {
				assert.Len(t, values["APP_SECRET"], 64)
			}
			assert.Len(t, config.RuntimeValues, 2, "caller-owned input must not be mutated")
		})
	}
}

func TestSSHScriptedInitializationValidatesBeforeInspection(t *testing.T) {
	for _, change := range []func(*deployment.InitializationConfig){
		func(c *deployment.InitializationConfig) { delete(c.RuntimeValues, "APP_URL") },
		func(c *deployment.InitializationConfig) { c.RuntimeValues["DATABASE_URL"] = "invalid-secret" },
		func(c *deployment.InitializationConfig) { c.RuntimeValues["UNKNOWN"] = "invalid-secret" },
		func(c *deployment.InitializationConfig) {
			c.InstallValues = map[string]string{"INSTALL_LOCALE": "en-GB"}
		},
		func(c *deployment.InitializationConfig) {
			c.InstallValues = map[string]string{"INSTALL_ADMIN_PASSWORD": "short"}
		},
		func(c *deployment.InitializationConfig) {
			c.InstallValues = map[string]string{"INSTALL_ADMIN_PASSWORD": "long-password", "INSTALL_ADMIN_EMAIL": "invalid-secret"}
		},
	} {
		config := scriptedInitializationConfig()
		change(config)
		store := &memoryInitializationStore{}
		err := (&SSH{initStore: store}).InitializeDeployment(t.Context(), deployment.InitializationOptions{Config: config})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "invalid-secret")
		assert.Zero(t, store.inspections)
		assert.Empty(t, store.writes)
	}
}

func TestSSHScriptedInitializationInstallDefaults(t *testing.T) {
	store := &memoryInitializationStore{}
	config := scriptedInitializationConfig()
	config.InstallValues = map[string]string{"INSTALL_ADMIN_PASSWORD": "long-password", "INSTALL_CURRENCY": "usd"}
	require.NoError(t, (&SSH{initStore: store}).InitializeDeployment(t.Context(), deployment.InitializationOptions{Config: config}))
	assert.Equal(t, map[string]string{"INSTALL_ADMIN_PASSWORD": "long-password", "INSTALL_CURRENCY": "USD", "INSTALL_LOCALE": "en-GB", "INSTALL_ADMIN_USERNAME": "admin"}, store.writes[0].InstallValues)
	assert.Len(t, config.InstallValues, 2)
}

func TestSSHGroupScriptedInitialization(t *testing.T) {
	group, stores := fixtureInitializationGroup(t)
	require.NoError(t, group.InitializeDeployment(t.Context(), deployment.InitializationOptions{Config: scriptedInitializationConfig()}))
	assert.Empty(t, stores["a"].writes)
	require.Len(t, stores["b"].writes, 1)
	assert.Equal(t, "https://shop.example.com", stores["b"].writes[0].RuntimeValues["APP_URL"])
}

func TestSSHScriptedInitializationWritesRemoteConfiguration(t *testing.T) {
	backend := localDeploymentSSH(t)
	config, err := deployment.ReadInitializationConfig(strings.NewReader(`{"runtime_values":{"APP_URL":"https://shop.example.com","DATABASE_URL":"mysql://user:secret@localhost/shop"},"install_values":{"INSTALL_ADMIN_PASSWORD":"long-password"}}`))
	require.NoError(t, err)
	ctx := system.WithInteraction(t.Context(), false)
	require.NoError(t, backend.InitializeDeployment(ctx, deployment.InitializationOptions{Config: config}))
	values, err := envfile.ReadValues(filepath.Join(filepath.Dir(backend.directory), "shared"), "APP_URL", "APP_SECRET", "DATABASE_URL")
	require.NoError(t, err)
	assert.Equal(t, config.RuntimeValues["APP_URL"], values["APP_URL"])
	assert.Equal(t, config.RuntimeValues["DATABASE_URL"], values["DATABASE_URL"])
	assert.Len(t, values["APP_SECRET"], 64)
}

func TestSSHInitializationWithoutInteractionRequiresConfig(t *testing.T) {
	store := &memoryInitializationStore{}
	err := (&SSH{initStore: store}).InitializeDeployment(system.WithInteraction(t.Context(), false), deployment.InitializationOptions{})
	require.Error(t, err)
	assert.Zero(t, store.inspections)
	assert.Empty(t, store.writes)
}

func TestSSHScriptedInitializationRepeatDoesNotWrite(t *testing.T) {
	store := &memoryInitializationStore{}
	store.states[0].Initialized = true
	require.NoError(t, (&SSH{initStore: store}).InitializeDeployment(t.Context(), deployment.InitializationOptions{Config: scriptedInitializationConfig()}))
	assert.Empty(t, store.writes)
}

package project

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/system"
)

func newDomainFlagCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("domain", "", "")
	cmd.SetContext(t.Context())
	return cmd
}

func TestResolveDomainFlag(t *testing.T) {
	t.Run("no flag returns the stored default", func(t *testing.T) {
		isolateProxyState(t)
		domain, change, err := resolveDomainFlag(newDomainFlagCommand(t))
		require.NoError(t, err)
		assert.Equal(t, "shopware.local", domain)
		assert.Nil(t, change)
	})

	t.Run("same domain is a no-op", func(t *testing.T) {
		isolateProxyState(t)
		cmd := newDomainFlagCommand(t)
		require.NoError(t, cmd.Flags().Set("domain", "shopware.local"))
		domain, change, err := resolveDomainFlag(cmd)
		require.NoError(t, err)
		assert.Equal(t, "shopware.local", domain)
		assert.Nil(t, change)
	})

	t.Run("invalid domain errors", func(t *testing.T) {
		isolateProxyState(t)
		cmd := newDomainFlagCommand(t)
		require.NoError(t, cmd.Flags().Set("domain", "Bad_Domain"))
		_, _, err := resolveDomainFlag(cmd)
		require.Error(t, err)
	})

	// This value flows into root-privileged resolver writes; refusing while
	// projects are registered protects their hostnames and certificates.
	t.Run("registered projects block a domain change", func(t *testing.T) {
		isolateProxyState(t)
		reg := proxy.Registry{Projects: []proxy.ProjectEntry{{ProjectRoot: t.TempDir(), Hostname: "shop.shopware.local"}}}
		require.NoError(t, reg.Save())

		cmd := newDomainFlagCommand(t)
		require.NoError(t, cmd.Flags().Set("domain", "dev.internal"))
		_, _, err := resolveDomainFlag(cmd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "teardown")
	})

	t.Run("clean domain change persists nothing", func(t *testing.T) {
		isolateProxyState(t)
		cmd := newDomainFlagCommand(t)
		require.NoError(t, cmd.Flags().Set("domain", "dev.internal"))
		domain, change, err := resolveDomainFlag(cmd)
		require.NoError(t, err)
		assert.Equal(t, "dev.internal", domain)
		require.NotNil(t, change)
		assert.Equal(t, "shopware.local", change.previous)
		assert.Equal(t, "dev.internal", change.requested)

		settings, err := proxy.LoadSettings()
		require.NoError(t, err)
		assert.Equal(t, "shopware.local", settings.BaseDomain())
	})
}

func TestChooseAutomaticSetupNeverPromptsNonInteractively(t *testing.T) {
	// stdin is not a terminal under go test, so the sudo path must never be
	// chosen; this is the "never sudo unprompted" guarantee for CI and agents.
	auto, err := chooseAutomaticSetup(t.Context(), "shopware.local", true)
	require.NoError(t, err)
	assert.False(t, auto)

	auto, err = chooseAutomaticSetup(system.WithInteraction(t.Context(), false), "shopware.local", false)
	require.NoError(t, err)
	assert.False(t, auto)
}

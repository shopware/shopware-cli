package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/proxy"
	"github.com/shopware/shopware-cli/internal/shop"
)

func TestSwitchProjectConfigURLsRemembersPreProxyState(t *testing.T) {
	t.Run("no config file leaves everything alone", func(t *testing.T) {
		dir := t.TempDir()
		env := &proxyEnvironment{configPath: filepath.Join(dir, ".shopware-project.yml"), canonicalRoot: dir}

		state := env.switchProjectConfigURLs(proxy.Registry{}, "https://shop.shopware.local")
		assert.Nil(t, state)
		assert.NoFileExists(t, env.configPath)
	})

	t.Run("existing url is remembered and rewritten", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".shopware-project.yml")
		require.NoError(t, os.WriteFile(configPath, []byte("url: http://127.0.0.1:8000\n"), 0o644))
		env := &proxyEnvironment{configPath: configPath, canonicalRoot: dir}

		state := env.switchProjectConfigURLs(proxy.Registry{}, "https://shop.shopware.local")
		require.NotNil(t, state)
		assert.True(t, state.HasFile)
		assert.True(t, state.HasRoot)
		assert.Equal(t, "http://127.0.0.1:8000", state.RootURL)

		after, err := shop.ReadProjectURLState(configPath, "")
		require.NoError(t, err)
		assert.Equal(t, "https://shop.shopware.local", after.RootURL)
	})

	// The up-to-down restore invariant: re-registration must return the state
	// remembered by the registry, not re-read the already-proxied file, or
	// "proxy down" could never restore the user's original URLs.
	t.Run("registry state wins over the rewritten file", func(t *testing.T) {
		dir := t.TempDir()
		configPath := filepath.Join(dir, ".shopware-project.yml")
		require.NoError(t, os.WriteFile(configPath, []byte("url: https://already-proxied.shopware.local\n"), 0o644))
		remembered := &shop.ConfigURLState{HasFile: true, HasRoot: true, RootURL: "http://127.0.0.1:8000"}
		reg := proxy.Registry{Projects: []proxy.ProjectEntry{{ProjectRoot: dir, PreviousConfig: remembered}}}
		env := &proxyEnvironment{configPath: configPath, canonicalRoot: dir}

		state := env.switchProjectConfigURLs(reg, "https://shop.shopware.local")
		assert.Same(t, remembered, state)
	})
}

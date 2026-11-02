package ssh

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestCachetoolInputDisabled(t *testing.T) {
	for _, env := range []*shop.EnvironmentConfig{
		nil,
		{},
		{SSH: &shop.EnvironmentSSHConfig{}},
		{SSH: &shop.EnvironmentSSHConfig{Cachetool: &shop.EnvironmentSSHCachetoolConfig{}}},
		{SSH: &shop.EnvironmentSSHConfig{Cachetool: &shop.EnvironmentSSHCachetoolConfig{Enabled: new(false), Adapter: "fcgi"}}},
	} {
		settings, err := (&SSH{env: env}).cachetoolInput()
		require.NoError(t, err)
		assert.Nil(t, settings)
	}
}

func TestCachetoolInputEnabled(t *testing.T) {
	for _, config := range []shop.EnvironmentSSHCachetoolConfig{
		{Enabled: new(true), Adapter: "fcgi", FCGI: "/run/php/php-fpm.sock", FCGIChroot: "/srv/shop", TmpDir: "/srv/tmp"},
		{Enabled: new(true), Adapter: "web", WebURL: "https://shop.example.com/cachetool", WebPath: "/srv/cachetool"},
		{Enabled: new(true), Config: "/etc/cachetool.yml"},
	} {
		backend := &SSH{env: &shop.EnvironmentConfig{SSH: &shop.EnvironmentSSHConfig{Cachetool: &config}}}
		settings, err := backend.cachetoolInput()
		require.NoError(t, err)
		require.NotNil(t, settings)
		assert.Equal(t, config, settings.EnvironmentSSHCachetoolConfig)
		assert.Positive(t, settings.TimeoutSeconds)
		assert.NotEmpty(t, settings.Version)
		assert.Contains(t, settings.URL, "/"+settings.Version+"/")
		assert.Regexp(t, `^[a-f0-9]{64}$`, settings.SHA256)
	}
}

func TestCachetoolInputRejectsInvalidConfigEvenWhenDisabled(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		backend := &SSH{env: &shop.EnvironmentConfig{SSH: &shop.EnvironmentSSHConfig{
			Cachetool: &shop.EnvironmentSSHCachetoolConfig{Enabled: new(enabled), Adapter: "cli"},
		}}}
		settings, err := backend.cachetoolInput()
		require.Error(t, err)
		assert.Nil(t, settings)
	}
}

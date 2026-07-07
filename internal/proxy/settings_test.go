package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadSettingsDefaults(t *testing.T) {
	settings, err := LoadSettings(t.TempDir())
	assert.NoError(t, err)
	assert.Equal(t, DefaultSettings(), settings)
}

func TestSettingsRoundtrip(t *testing.T) {
	dir := t.TempDir()

	settings := DefaultSettings()
	settings.Domain = "shopware.internal"
	settings.HTTPSPort = 8443
	settings.Hosts = []string{"shop.example.test"}

	assert.NoError(t, SaveSettings(dir, settings))

	loaded, err := LoadSettings(dir)
	assert.NoError(t, err)
	assert.Equal(t, settings, loaded)
}

func TestRegisterHost(t *testing.T) {
	settings := DefaultSettings()

	assert.False(t, settings.RegisterHost("my-shop."+DefaultDomain), "covered by wildcard")
	assert.True(t, settings.RegisterHost("shop.example.test"))
	assert.False(t, settings.RegisterHost("shop.example.test"), "already registered")
	assert.Equal(t, []string{"shop.example.test"}, settings.Hosts)
}

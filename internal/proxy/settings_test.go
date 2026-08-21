package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSettingsBaseDomain(t *testing.T) {
	t.Parallel()

	assert.Equal(t, DefaultDomain, Settings{}.BaseDomain())
	assert.Equal(t, "dev.internal", Settings{Domain: "dev.internal"}.BaseDomain())
}

func TestValidateDomain(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{"shopware.local", "dev.internal", "shops.test", "a.b-c.d", "internal"} {
		assert.NoError(t, ValidateDomain(valid), valid)
	}

	for _, invalid := range []string{
		"",
		"Shopware.Local",
		"https://shopware.local",
		"shopware.local/path",
		".shopware.local",
		"shopware..local",
		"shop_ware.local",
		"-shopware.local",
	} {
		assert.Error(t, ValidateDomain(invalid), invalid)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	withTempStateDir(t)

	// A missing file yields the defaults.
	settings, err := LoadSettings()
	assert.NoError(t, err)
	assert.Equal(t, DefaultDomain, settings.BaseDomain())

	settings.Domain = "dev.internal"
	assert.NoError(t, SaveSettings(settings))

	loaded, err := LoadSettings()
	assert.NoError(t, err)
	assert.Equal(t, "dev.internal", loaded.BaseDomain())
}

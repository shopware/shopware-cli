package devtui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestNewConfigModel_NilConfig(t *testing.T) {
	m := NewConfigModel(nil)

	assert.Equal(t, 1, m.phpVersion)   // default 8.3 (index 1)
	assert.Equal(t, 0, m.nodeVersion)  // default 22 (index 0)
	assert.Equal(t, 0, m.profiler)     // default none (index 0)
	assert.False(t, m.editing)
	assert.False(t, m.saved)
	assert.False(t, m.modified)
}

func TestNewConfigModel_WithConfig(t *testing.T) {
	cfg := &shop.Config{
		Docker: &shop.ConfigDocker{
			PHP: &shop.ConfigDockerPHP{
				Version:           "8.2",
				Profiler:          "blackfire",
				BlackfireServerID: "my-server-id",
			},
			Node: &shop.ConfigDockerNode{
				Version: "24",
			},
		},
	}

	m := NewConfigModel(cfg)

	assert.Equal(t, 0, m.phpVersion) // 8.2 is index 0
	assert.Equal(t, 1, m.nodeVersion) // 24 is index 1
	assert.Equal(t, 2, m.profiler) // blackfire is index 2
	assert.Equal(t, "my-server-id", m.blackfireServerID.Value())
}

func TestConfigModel_ApplyToConfig(t *testing.T) {
	cfg := &shop.Config{}
	m := NewConfigModel(nil)
	m.phpVersion = 2  // 8.4
	m.nodeVersion = 1 // 24
	m.profiler = 1    // xdebug

	m.ApplyToConfig(cfg)

	assert.Equal(t, "8.4", cfg.Docker.PHP.Version)
	assert.Equal(t, "24", cfg.Docker.Node.Version)
	assert.Equal(t, "xdebug", cfg.Docker.PHP.Profiler)
	assert.Empty(t, cfg.Docker.PHP.BlackfireServerID)
}

func TestConfigModel_ApplyToConfig_Blackfire(t *testing.T) {
	cfg := &shop.Config{}
	m := NewConfigModel(nil)
	m.profiler = 2 // blackfire
	m.blackfireServerID.SetValue("srv-id")
	m.blackfireServerToken.SetValue("srv-token")

	m.ApplyToConfig(cfg)

	assert.Equal(t, "blackfire", cfg.Docker.PHP.Profiler)
	assert.Equal(t, "srv-id", cfg.Docker.PHP.BlackfireServerID)
	assert.Equal(t, "srv-token", cfg.Docker.PHP.BlackfireServerToken)
	assert.Empty(t, cfg.Docker.PHP.TidewaysAPIKey)
}

func TestConfigModel_ApplyToConfig_Tideways(t *testing.T) {
	cfg := &shop.Config{}
	m := NewConfigModel(nil)
	m.profiler = 3 // tideways
	m.tidewaysAPIKey.SetValue("tw-key")

	m.ApplyToConfig(cfg)

	assert.Equal(t, "tideways", cfg.Docker.PHP.Profiler)
	assert.Equal(t, "tw-key", cfg.Docker.PHP.TidewaysAPIKey)
	assert.Empty(t, cfg.Docker.PHP.BlackfireServerID)
}

func TestConfigModel_FieldVisibility(t *testing.T) {
	m := NewConfigModel(nil)

	// No profiler - credential fields should be hidden.
	m.profiler = 0
	assert.False(t, m.isFieldVisible(fieldBlackfireServerID))
	assert.False(t, m.isFieldVisible(fieldBlackfireServerToken))
	assert.False(t, m.isFieldVisible(fieldTidewaysAPIKey))

	// Blackfire - only blackfire fields visible.
	m.profiler = indexOf(profilers, "blackfire", 0)
	assert.True(t, m.isFieldVisible(fieldBlackfireServerID))
	assert.True(t, m.isFieldVisible(fieldBlackfireServerToken))
	assert.False(t, m.isFieldVisible(fieldTidewaysAPIKey))

	// Tideways - only tideways field visible.
	m.profiler = indexOf(profilers, "tideways", 0)
	assert.False(t, m.isFieldVisible(fieldBlackfireServerID))
	assert.True(t, m.isFieldVisible(fieldTidewaysAPIKey))
}

func TestConfigModel_CursorNavigation(t *testing.T) {
	m := NewConfigModel(nil)
	m.profiler = 0 // no profiler, so credential fields are hidden.

	assert.Equal(t, fieldPHPVersion, m.cursor)

	m.moveCursorDown()
	assert.Equal(t, fieldProfiler, m.cursor)

	// Should skip hidden credential fields to Node Version.
	m.moveCursorDown()
	assert.Equal(t, fieldNodeVersion, m.cursor, "should skip hidden fields to Node Version")

	m.moveCursorDown()
	assert.Equal(t, fieldSave, m.cursor)

	m.moveCursorUp()
	assert.Equal(t, fieldNodeVersion, m.cursor, "should go back to Node Version")

	m.moveCursorUp()
	assert.Equal(t, fieldProfiler, m.cursor, "should skip hidden fields going up")
}

func TestConfigModel_CycleValues(t *testing.T) {
	m := NewConfigModel(nil)

	initial := m.phpVersion
	m.cycleNext()
	assert.NotEqual(t, initial, m.phpVersion)
	assert.True(t, m.modified)

	m.modified = false
	m.cursor = fieldNodeVersion
	m.cycleNext()
	assert.True(t, m.modified)
}

func TestIndexOf(t *testing.T) {
	assert.Equal(t, 0, indexOf(phpVersions, "8.2", 1))
	assert.Equal(t, 1, indexOf(phpVersions, "8.3", 0))
	assert.Equal(t, 2, indexOf(phpVersions, "8.4", 0))
	assert.Equal(t, 1, indexOf(phpVersions, "unknown", 1)) // default
}

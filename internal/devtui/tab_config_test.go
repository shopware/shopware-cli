package devtui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestNewConfigModel_NilConfig(t *testing.T) {
	m := NewConfigModel(nil, "")

	assert.Equal(t, 1, m.phpVersion)
	assert.Equal(t, 0, m.profiler)
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
		},
	}

	m := NewConfigModel(cfg, "")

	assert.Equal(t, 0, m.phpVersion)
	assert.Equal(t, 2, m.profiler)
	assert.Equal(t, "my-server-id", m.blackfireServerID.Value())
}

func TestConfigModel_ApplyToConfig(t *testing.T) {
	cfg := &shop.Config{}
	m := NewConfigModel(nil, "")
	m.phpVersion = 2
	m.profiler = 1

	m.ApplyToConfig(cfg)

	assert.Equal(t, "8.4", cfg.Docker.PHP.Version)
	assert.Equal(t, "xdebug", cfg.Docker.PHP.Profiler)
	assert.Empty(t, cfg.Docker.PHP.BlackfireServerID)
}

func TestConfigModel_ApplyToConfig_BlackfireCredentialsExcluded(t *testing.T) {
	cfg := &shop.Config{}
	m := NewConfigModel(nil, "")
	m.profiler = 2
	m.blackfireServerID.SetValue("srv-id")
	m.blackfireServerToken.SetValue("srv-token")

	m.ApplyToConfig(cfg)

	assert.Equal(t, "blackfire", cfg.Docker.PHP.Profiler)
	assert.Empty(t, cfg.Docker.PHP.BlackfireServerID)
	assert.Empty(t, cfg.Docker.PHP.BlackfireServerToken)
}

func TestConfigModel_LocalConfig_Blackfire(t *testing.T) {
	m := NewConfigModel(nil, "")
	m.profiler = 2
	m.blackfireServerID.SetValue("srv-id")
	m.blackfireServerToken.SetValue("srv-token")

	localCfg := m.LocalConfig()
	assert.NotNil(t, localCfg)
	assert.Equal(t, "srv-id", localCfg.Docker.PHP.BlackfireServerID)
	assert.Equal(t, "srv-token", localCfg.Docker.PHP.BlackfireServerToken)
}

func TestConfigModel_LocalConfig_Tideways(t *testing.T) {
	m := NewConfigModel(nil, "")
	m.profiler = 3
	m.tidewaysAPIKey.SetValue("tw-key")

	localCfg := m.LocalConfig()
	assert.NotNil(t, localCfg)
	assert.Equal(t, "tw-key", localCfg.Docker.PHP.TidewaysAPIKey)
	assert.Empty(t, localCfg.Docker.PHP.BlackfireServerID)
}

func TestConfigModel_LocalConfig_NoCredentials(t *testing.T) {
	m := NewConfigModel(nil, "")
	m.profiler = 0

	assert.Nil(t, m.LocalConfig())

	m.profiler = 1
	assert.Nil(t, m.LocalConfig())
}

func TestConfigModel_FieldVisibility(t *testing.T) {
	m := NewConfigModel(nil, "")

	m.profiler = 0
	assert.False(t, m.isFieldVisible(fieldBlackfireServerID))
	assert.False(t, m.isFieldVisible(fieldBlackfireServerToken))
	assert.False(t, m.isFieldVisible(fieldTidewaysAPIKey))

	m.profiler = indexOf(profilers, "blackfire", 0)
	assert.True(t, m.isFieldVisible(fieldBlackfireServerID))
	assert.True(t, m.isFieldVisible(fieldBlackfireServerToken))
	assert.False(t, m.isFieldVisible(fieldTidewaysAPIKey))

	m.profiler = indexOf(profilers, "tideways", 0)
	assert.False(t, m.isFieldVisible(fieldBlackfireServerID))
	assert.True(t, m.isFieldVisible(fieldTidewaysAPIKey))
}

func TestConfigModel_CursorNavigation(t *testing.T) {
	m := NewConfigModel(nil, "")
	m.profiler = 0

	assert.Equal(t, fieldAppEnv, m.cursor)

	m.moveCursorDown()
	assert.Equal(t, fieldPHPVersion, m.cursor)

	m.moveCursorDown()
	assert.Equal(t, fieldProfiler, m.cursor)

	m.moveCursorDown()
	assert.Equal(t, fieldSave, m.cursor)

	m.moveCursorUp()
	assert.Equal(t, fieldProfiler, m.cursor)
}

func TestConfigModel_CursorNavigation_BlackfireVisible(t *testing.T) {
	m := NewConfigModel(nil, "")
	m.profiler = indexOf(profilers, "blackfire", 0)

	assert.Equal(t, fieldAppEnv, m.cursor)
	m.moveCursorDown()
	assert.Equal(t, fieldPHPVersion, m.cursor)
	m.moveCursorDown()
	assert.Equal(t, fieldProfiler, m.cursor)
	m.moveCursorDown()
	assert.Equal(t, fieldBlackfireServerID, m.cursor)
	m.moveCursorDown()
	assert.Equal(t, fieldBlackfireServerToken, m.cursor)
	m.moveCursorDown()
	assert.Equal(t, fieldSave, m.cursor)
}

func TestConfigModel_PickerForCursor_Select(t *testing.T) {
	m := NewConfigModel(nil, "")
	m.cursor = fieldPHPVersion

	modal := m.PickerForCursor()
	picker, ok := modal.(*listPicker)
	assert.True(t, ok)
	assert.Equal(t, fieldPHPVersion, picker.key)
	assert.Len(t, picker.items, len(phpVersions))
	values := make([]string, len(picker.items))
	for i, it := range picker.items {
		values[i] = it.Value
	}
	assert.ElementsMatch(t, phpVersions, values)
}

func TestConfigModel_PickerForCursor_Text(t *testing.T) {
	m := NewConfigModel(nil, "")
	m.profiler = indexOf(profilers, "blackfire", 0)
	m.blackfireServerID.SetValue("existing")
	m.cursor = fieldBlackfireServerID

	modal := m.PickerForCursor()
	picker, ok := modal.(*textPicker)
	assert.True(t, ok)
	assert.Equal(t, "existing", picker.input.Value())
}

func TestConfigModel_ApplyPickerValue(t *testing.T) {
	m := NewConfigModel(nil, "")
	m.modified = false

	changed := m.ApplyPickerValue(fieldPHPVersion, "8.4")
	assert.True(t, changed)
	assert.Equal(t, indexOf(phpVersions, "8.4", -1), m.phpVersion)
	assert.True(t, m.modified)

	m.modified = false
	changed = m.ApplyPickerValue(fieldPHPVersion, "8.4")
	assert.False(t, changed)
	assert.False(t, m.modified)
}

func TestConfigModel_ApplyPickerValue_TextField(t *testing.T) {
	m := NewConfigModel(nil, "")
	m.profiler = indexOf(profilers, "tideways", 0)

	changed := m.ApplyPickerValue(fieldTidewaysAPIKey, "secret-key")
	assert.True(t, changed)
	assert.Equal(t, "secret-key", m.tidewaysAPIKey.Value())
	assert.True(t, m.modified)
}

func TestConfigModel_AppEnv_DefaultsToDev(t *testing.T) {
	m := NewConfigModel(nil, "")

	assert.Equal(t, "dev", m.AppEnv())
	assert.False(t, m.AppEnvChanged())
}

func TestConfigModel_AppEnv_LoadedFromExisting(t *testing.T) {
	m := NewConfigModel(nil, "prod")

	assert.Equal(t, "prod", m.AppEnv())
	assert.False(t, m.AppEnvChanged())
}

func TestConfigModel_AppEnv_PickerOptions(t *testing.T) {
	m := NewConfigModel(nil, "dev")
	m.cursor = fieldAppEnv

	modal := m.PickerForCursor()
	picker, ok := modal.(*listPicker)
	assert.True(t, ok)
	assert.Equal(t, fieldAppEnv, picker.key)
	values := make([]string, len(picker.items))
	for i, it := range picker.items {
		values[i] = it.Value
	}
	assert.ElementsMatch(t, appEnvs, values)
}

func TestConfigModel_AppEnv_ApplyPickerValue(t *testing.T) {
	m := NewConfigModel(nil, "dev")

	changed := m.ApplyPickerValue(fieldAppEnv, "prod")
	assert.True(t, changed)
	assert.Equal(t, "prod", m.AppEnv())
	assert.True(t, m.AppEnvChanged())
	assert.True(t, m.modified)

	m.modified = false
	changed = m.ApplyPickerValue(fieldAppEnv, "prod")
	assert.False(t, changed)
	assert.False(t, m.modified)
}

func TestConfigModel_AppEnv_MarkPersistedClearsChange(t *testing.T) {
	m := NewConfigModel(nil, "dev")
	m.ApplyPickerValue(fieldAppEnv, "prod")
	assert.True(t, m.AppEnvChanged())

	m.MarkAppEnvPersisted()
	assert.False(t, m.AppEnvChanged())
}

func TestIndexOf(t *testing.T) {
	assert.Equal(t, 0, indexOf(phpVersions, "8.2", 1))
	assert.Equal(t, 1, indexOf(phpVersions, "8.3", 0))
	assert.Equal(t, 2, indexOf(phpVersions, "8.4", 0))
	assert.Equal(t, 1, indexOf(phpVersions, "unknown", 1))
}

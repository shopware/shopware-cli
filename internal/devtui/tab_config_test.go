package devtui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestNewConfigModel_NilConfig(t *testing.T) {
	m := NewConfigModel(nil, nil)

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

	m := NewConfigModel(cfg, nil)

	assert.Equal(t, 0, m.phpVersion)
	assert.Equal(t, 2, m.profiler)
	assert.Equal(t, "my-server-id", m.blackfireServerID.Value())
}

func TestConfigModel_ApplyToConfig(t *testing.T) {
	cfg := &shop.Config{}
	m := NewConfigModel(nil, nil)
	m.phpVersion = 2
	m.profiler = 1

	m.ApplyToConfig(cfg)

	assert.Equal(t, "8.4", cfg.Docker.PHP.Version)
	assert.Equal(t, "xdebug", cfg.Docker.PHP.Profiler)
	assert.Empty(t, cfg.Docker.PHP.BlackfireServerID)
}

func TestConfigModel_ApplyToConfig_BlackfireCredentialsExcluded(t *testing.T) {
	cfg := &shop.Config{}
	m := NewConfigModel(nil, nil)
	m.profiler = 2
	m.blackfireServerID.SetValue("srv-id")
	m.blackfireServerToken.SetValue("srv-token")

	m.ApplyToConfig(cfg)

	assert.Equal(t, "blackfire", cfg.Docker.PHP.Profiler)
	assert.Empty(t, cfg.Docker.PHP.BlackfireServerID)
	assert.Empty(t, cfg.Docker.PHP.BlackfireServerToken)
}

func TestConfigModel_LocalConfig_Blackfire(t *testing.T) {
	m := NewConfigModel(nil, nil)
	m.profiler = 2
	m.blackfireServerID.SetValue("srv-id")
	m.blackfireServerToken.SetValue("srv-token")

	localCfg := m.LocalConfig()
	assert.NotNil(t, localCfg)
	assert.Equal(t, "srv-id", localCfg.Docker.PHP.BlackfireServerID)
	assert.Equal(t, "srv-token", localCfg.Docker.PHP.BlackfireServerToken)
}

func TestConfigModel_LocalConfig_Tideways(t *testing.T) {
	m := NewConfigModel(nil, nil)
	m.profiler = 3
	m.tidewaysAPIKey.SetValue("tw-key")

	localCfg := m.LocalConfig()
	assert.NotNil(t, localCfg)
	assert.Equal(t, "tw-key", localCfg.Docker.PHP.TidewaysAPIKey)
	assert.Empty(t, localCfg.Docker.PHP.BlackfireServerID)
}

func TestConfigModel_LocalConfig_NoCredentials(t *testing.T) {
	m := NewConfigModel(nil, nil)
	m.profiler = 0

	assert.Nil(t, m.LocalConfig())

	m.profiler = 1
	assert.Nil(t, m.LocalConfig())
}

func TestConfigModel_FieldVisibility(t *testing.T) {
	m := NewConfigModel(nil, nil)

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
	m := NewConfigModel(nil, nil)
	m.profiler = 0

	assert.Equal(t, fieldAppEnv, m.cursor)

	m.moveCursorDown()
	assert.Equal(t, fieldHTTPCache, m.cursor)

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
	m := NewConfigModel(nil, nil)
	m.profiler = indexOf(profilers, "blackfire", 0)

	assert.Equal(t, fieldAppEnv, m.cursor)
	m.moveCursorDown()
	assert.Equal(t, fieldHTTPCache, m.cursor)
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
	m := NewConfigModel(nil, nil)
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

func TestConfigModel_PickerForCursor_ProfilerFreePaidLabels(t *testing.T) {
	m := NewConfigModel(nil, nil)
	m.cursor = fieldProfiler

	modal := m.PickerForCursor()
	picker, ok := modal.(*listPicker)
	assert.True(t, ok)

	details := make(map[string]string, len(picker.items))
	for _, it := range picker.items {
		details[it.Value] = it.Detail
	}

	assert.Equal(t, "", details[""])              // none has no free/paid badge
	assert.Equal(t, "free", details["xdebug"])    // open source
	assert.Equal(t, "free", details["pcov"])      // open source
	assert.Equal(t, "free", details["spx"])       // open source
	assert.Equal(t, "paid", details["blackfire"]) // commercial SaaS
	assert.Equal(t, "paid", details["tideways"])  // commercial SaaS
}

func TestConfigModel_PickerForCursor_Text(t *testing.T) {
	m := NewConfigModel(nil, nil)
	m.profiler = indexOf(profilers, "blackfire", 0)
	m.blackfireServerID.SetValue("existing")
	m.cursor = fieldBlackfireServerID

	modal := m.PickerForCursor()
	picker, ok := modal.(*textPicker)
	assert.True(t, ok)
	assert.Equal(t, "existing", picker.input.Value())
}

func TestConfigModel_ApplyPickerValue(t *testing.T) {
	m := NewConfigModel(nil, nil)
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
	m := NewConfigModel(nil, nil)
	m.profiler = indexOf(profilers, "tideways", 0)

	changed := m.ApplyPickerValue(fieldTidewaysAPIKey, "secret-key")
	assert.True(t, changed)
	assert.Equal(t, "secret-key", m.tidewaysAPIKey.Value())
	assert.True(t, m.modified)
}

func TestConfigModel_EnvField_DefaultsApplied(t *testing.T) {
	m := NewConfigModel(nil, nil)

	assert.Equal(t, "dev", m.EnvValue("APP_ENV"))
	assert.Empty(t, m.ChangedEnvValues())
}

func TestConfigModel_EnvField_LoadedFromValues(t *testing.T) {
	m := NewConfigModel(nil, map[string]string{"APP_ENV": "prod"})

	assert.Equal(t, "prod", m.EnvValue("APP_ENV"))
	assert.Empty(t, m.ChangedEnvValues())
}

func TestConfigModel_EnvField_PickerOptionsDrivenByRegistry(t *testing.T) {
	m := NewConfigModel(nil, map[string]string{"APP_ENV": "dev"})
	m.cursor = fieldAppEnv

	modal := m.PickerForCursor()
	picker, ok := modal.(*listPicker)
	assert.True(t, ok)
	assert.Equal(t, fieldAppEnv, picker.key)

	def, ok := envFieldByConfigField(fieldAppEnv)
	assert.True(t, ok)
	values := make([]string, len(picker.items))
	for i, it := range picker.items {
		values[i] = it.Value
	}
	assert.ElementsMatch(t, def.choices, values)
}

func TestConfigModel_EnvField_ApplyPickerValue(t *testing.T) {
	m := NewConfigModel(nil, map[string]string{"APP_ENV": "dev"})

	changed := m.ApplyPickerValue(fieldAppEnv, "prod")
	assert.True(t, changed)
	assert.Equal(t, "prod", m.EnvValue("APP_ENV"))
	assert.Equal(t, map[string]string{"APP_ENV": "prod"}, m.ChangedEnvValues())
	assert.True(t, m.modified)

	m.modified = false
	changed = m.ApplyPickerValue(fieldAppEnv, "prod")
	assert.False(t, changed)
	assert.False(t, m.modified)
}

func TestConfigModel_EnvField_MarkPersistedClearsChange(t *testing.T) {
	m := NewConfigModel(nil, map[string]string{"APP_ENV": "dev"})
	m.ApplyPickerValue(fieldAppEnv, "prod")
	assert.NotEmpty(t, m.ChangedEnvValues())

	m.MarkEnvValuesPersisted()
	assert.Empty(t, m.ChangedEnvValues())
}

func TestEnvFieldKeys_IncludesAppEnv(t *testing.T) {
	assert.Contains(t, EnvFieldKeys(), "APP_ENV")
}

func TestConfigModel_HTTPCache_DefaultsToDisabled(t *testing.T) {
	m := NewConfigModel(nil, nil)

	assert.Equal(t, "0", m.EnvValue("SHOPWARE_HTTP_CACHE_ENABLED"))
	assert.Empty(t, m.ChangedEnvValues())
}

func TestConfigModel_HTTPCache_ToggleProducesEnvChange(t *testing.T) {
	m := NewConfigModel(nil, map[string]string{"SHOPWARE_HTTP_CACHE_ENABLED": "0"})

	changed := m.ApplyPickerValue(fieldHTTPCache, "1")
	assert.True(t, changed)
	assert.Equal(t, "1", m.EnvValue("SHOPWARE_HTTP_CACHE_ENABLED"))
	assert.Equal(t, map[string]string{"SHOPWARE_HTTP_CACHE_ENABLED": "1"}, m.ChangedEnvValues())
}

func TestConfigModel_HTTPCache_PickerUsesFriendlyLabels(t *testing.T) {
	m := NewConfigModel(nil, nil)
	m.cursor = fieldHTTPCache

	modal := m.PickerForCursor()
	picker, ok := modal.(*listPicker)
	assert.True(t, ok)

	labels := make([]string, len(picker.items))
	values := make([]string, len(picker.items))
	for i, it := range picker.items {
		labels[i] = it.Label
		values[i] = it.Value
	}
	assert.Equal(t, []string{"disabled", "enabled"}, labels)
	assert.Equal(t, []string{"0", "1"}, values)
}

func TestEnvFieldKeys_IncludesHTTPCache(t *testing.T) {
	assert.Contains(t, EnvFieldKeys(), "SHOPWARE_HTTP_CACHE_ENABLED")
}

func TestIndexOf(t *testing.T) {
	assert.Equal(t, 0, indexOf(phpVersions, "8.2", 1))
	assert.Equal(t, 1, indexOf(phpVersions, "8.3", 0))
	assert.Equal(t, 2, indexOf(phpVersions, "8.4", 0))
	assert.Equal(t, 1, indexOf(phpVersions, "unknown", 1))
}

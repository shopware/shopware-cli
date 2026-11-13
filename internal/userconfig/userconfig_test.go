package userconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	t.Setenv(PathEnv, path)
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.yaml")
	t.Setenv(PathEnv, path)

	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.TelemetryEnabled())
	assert.False(t, cfg.NoUpdateHint)
	assert.NoFileExists(t, path)
}

func TestLoadDefaultPath(t *testing.T) {
	t.Setenv(PathEnv, "")
	configDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("no user config dir")
	}

	path, err := Path()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(configDir, "shopware-cli", "config.yaml"), path)
}

func TestLoadKeys(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		telemetry    bool
		noUpdateHint bool
	}{
		{name: "empty file", content: "", telemetry: true},
		{name: "telemetry disabled", content: "telemetry: false\n", telemetry: false},
		{name: "telemetry enabled", content: "telemetry: true\n", telemetry: true},
		{name: "update hint disabled", content: "no_update_hint: true\n", telemetry: true, noUpdateHint: true},
		{name: "both keys", content: "telemetry: false\nno_update_hint: true\n", telemetry: false, noUpdateHint: true},
		{name: "unknown keys ignored", content: "verbose: true\n", telemetry: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeConfig(t, tt.content)

			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, tt.telemetry, cfg.TelemetryEnabled())
			assert.Equal(t, tt.noUpdateHint, cfg.NoUpdateHint)
		})
	}
}

func TestLoadInvalidFileReturnsDefaultsAndError(t *testing.T) {
	writeConfig(t, "telemetry: [\n")

	cfg, err := Load()
	assert.Error(t, err)
	assert.True(t, cfg.TelemetryEnabled())
	assert.False(t, cfg.NoUpdateHint)
}

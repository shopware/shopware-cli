package shop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectShopwareVersion(t *testing.T) {
	const locked = `{"packages":[{"name":"shopware/core","version":"v6.7.2.0"}]}`
	for _, scenario := range []struct {
		name, installed, lock, expected string
	}{
		{"installed takes precedence", `{"packages":[{"name":"shopware/core","version":"v6.7.1.0"}]}`, locked, "6.7.1.0"},
		{"legacy installed format", `[{"name":"shopware/core","version":"6.6.1.0"}]`, "", "6.6.1.0"},
		{"installed platform", `{"packages":[{"name":"shopware/platform","version":"v6.7.1.0"}]}`, "", "6.7.1.0"},
		{"lock without vendor", "", locked, "6.7.2.0"},
		{"invalid installed falls back", "invalid", locked, "6.7.2.0"},
		{"missing installed package falls back", `{"packages":[]}`, locked, "6.7.2.0"},
		{"locked platform", "", `{"packages":[{"name":"shopware/platform","version":"v6.7.0.0"}]}`, "6.7.0.0"},
		{"missing files", "", "", ""},
		{"invalid lock", "", "invalid", ""},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			if scenario.installed != "" {
				installedDir := filepath.Join(root, "vendor", "composer")
				require.NoError(t, os.MkdirAll(installedDir, 0o755))
				require.NoError(t, os.WriteFile(filepath.Join(installedDir, "installed.json"), []byte(scenario.installed), 0o644))
			}
			if scenario.lock != "" {
				require.NoError(t, os.WriteFile(filepath.Join(root, "composer.lock"), []byte(scenario.lock), 0o644))
			}
			assert.Equal(t, scenario.expected, DetectShopwareVersion(root))
		})
	}
}

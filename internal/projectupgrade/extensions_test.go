package projectupgrade

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposerOperationsParsesUpgradesAndAbandoned(t *testing.T) {
	t.Parallel()

	output := []string{
		"Loading composer repositories with package information",
		"Lock file operations: 1 install, 3 updates, 0 removals",
		"  - Upgrading shopware/core (v6.5.8.0 => v6.6.4.0)",
		"  - Upgrading swag/paypal (v8.0.0 => v9.2.0)",
		"  - Downgrading acme/downgraded (2.0.0 => 1.9.0)",
		"  - Installing new/dependency (1.0.0)",
		"Package swag/legacy is abandoned, you should avoid using it. Use swag/replacement instead.",
		"Package acme/dead is abandoned, you should avoid using it. No replacement was suggested.",
	}

	plugins := map[string]string{
		"swag/paypal":     "8.0.0",
		"acme/downgraded": "2.0.0",
		"swag/legacy":     "1.0.0",
		"acme/dead":       "3.0.0",
	}

	targets, abandoned := composerOperations(output, plugins)

	assert.Equal(t, map[string]string{
		"swag/paypal":     "9.2.0",
		"acme/downgraded": "1.9.0",
	}, targets)
	assert.Equal(t, map[string]string{
		"swag/legacy": "swag/replacement",
		"acme/dead":   "",
	}, abandoned)
}

func TestComposerOperationsIgnoresNonPluginPackages(t *testing.T) {
	t.Parallel()

	output := []string{
		"  - Upgrading shopware/core (v6.5.8.0 => v6.6.4.0)",
		"Package symfony/old is abandoned, you should avoid using it. No replacement was suggested.",
	}

	targets, abandoned := composerOperations(output, map[string]string{"swag/paypal": "8.0.0"})
	assert.Empty(t, targets)
	assert.Empty(t, abandoned)
}

func TestBuildExtensionQueueStatesAndRiskOrder(t *testing.T) {
	t.Parallel()

	installed := map[string]string{
		"swag/ok":         "1.0.0",
		"swag/update":     "2.0.0",
		"swag/blocked":    "3.0.0",
		"swag/deprecated": "4.0.0",
	}

	report := CompatReport{
		OK: false,
		Output: []string{
			"  - Upgrading swag/update (v2.0.0 => v2.5.0)",
			"Package swag/deprecated is abandoned, you should avoid using it. Use swag/new instead.",
		},
		BlockingPlugins: []string{"swag/blocked"},
	}

	rows := BuildExtensionQueue(installed, report)
	require.Len(t, rows, 4)

	// Risk order: blocked, deprecated, update, ok.
	assert.Equal(t, "swag/blocked", rows[0].Name)
	assert.Equal(t, ExtensionBlocked, rows[0].State)
	assert.Empty(t, rows[0].Target)

	assert.Equal(t, "swag/deprecated", rows[1].Name)
	assert.Equal(t, ExtensionDeprecated, rows[1].State)
	assert.Equal(t, "swag/new", rows[1].Replacement)

	assert.Equal(t, "swag/update", rows[2].Name)
	assert.Equal(t, ExtensionUpdate, rows[2].State)
	assert.Equal(t, "2.5.0", rows[2].Target)

	assert.Equal(t, "swag/ok", rows[3].Name)
	assert.Equal(t, ExtensionOK, rows[3].State)
	assert.Equal(t, "1.0.0", rows[3].Target, "unchanged extensions keep their version as target")
}

func TestBuildExtensionQueueAllResolvable(t *testing.T) {
	t.Parallel()

	installed := map[string]string{"swag/a": "1.0.0", "swag/b": "1.1.0"}
	report := CompatReport{OK: true, Output: []string{"  - Upgrading swag/b (v1.1.0 => v1.2.0)"}}

	rows := BuildExtensionQueue(installed, report)
	require.Len(t, rows, 2)
	assert.Equal(t, 0, CountBlockers(rows))
	assert.Equal(t, ExtensionUpdate, rows[0].State)
	assert.Equal(t, "swag/b", rows[0].Name)
	assert.Equal(t, ExtensionOK, rows[1].State)
}

func TestRequiredPluginVersions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJson := filepath.Join(dir, "composer.json")
	require.NoError(t, os.WriteFile(composerJson, []byte(`{
		"require": {
			"shopware/core": "6.5.8.0",
			"swag/paypal": "*",
			"swag/unrelated-lib": "*"
		}
	}`), 0o644))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor", "composer"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "composer", "installed.json"), []byte(`{
		"packages": [
			{"name": "swag/paypal", "type": "shopware-platform-plugin", "version": "v8.0.0"},
			{"name": "swag/not-required", "type": "shopware-platform-plugin", "version": "v1.0.0"},
			{"name": "swag/unrelated-lib", "type": "library", "version": "v2.0.0"}
		]
	}`), 0o644))

	plugins, err := RequiredPluginVersions(composerJson)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"swag/paypal": "8.0.0"}, plugins)
}

package projectupgrade

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/packagist"
)

func TestBlockingPluginsAttributesOnlyRequestedPlugins(t *testing.T) {
	t.Parallel()

	output := []string{
		"Loading composer repositories with package information",
		"Your requirements could not be resolved to an installable set of packages.",
		"  Problem 1",
		"    - swag/paypal 9.0.0 requires shopware/core ^6.7 -> found shopware/core[6.6.4.0]",
		"    - root composer.json requires swag/paypal ^9.0",
	}

	// shopware/core appears in the output too, but it is not a plugin we asked
	// for, so it must never be attributed as a blocker.
	blockers := blockingPlugins(output, []string{"swag/paypal", "frosh/tools"})

	assert.Equal(t, []string{"swag/paypal"}, blockers)
}

func TestBlockingPluginsEmptyWhenNoPluginNamed(t *testing.T) {
	t.Parallel()

	output := []string{"Your requirements could not be resolved", "ext-foo is missing"}
	assert.Empty(t, blockingPlugins(output, []string{"swag/paypal"}))
}

func TestFilterPackagesDropsOnlyDroppedNames(t *testing.T) {
	t.Parallel()

	pkgs := []string{"shopware/core:6.6.4.0", "swag/paypal", "frosh/tools"}
	dropped := map[string]struct{}{"swag/paypal": {}}

	assert.Equal(t, []string{"shopware/core:6.6.4.0", "frosh/tools"}, filterPackages(pkgs, dropped))
}

func TestRequirePackagesPinsCoreAndListsPlugins(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	composerJsonPath := filepath.Join(dir, "composer.json")

	writeJSON(t, composerJsonPath, map[string]any{
		"name": "shopware/production",
		"require": map[string]any{
			"shopware/core":       "6.5.8.0",
			"shopware/storefront": "6.5.8.0",
			"swag/paypal":         "^8.11",
			"frosh/tools":         "^1.4",
			"unrelated/library":   "^1.0",
		},
	})

	writeInstalledJSON(t, dir, []packagist.InstalledPackage{
		{Name: "swag/paypal", Type: composerPluginType, InstallPath: "../swag/paypal"},
		{Name: "frosh/tools", Type: composerPluginType, InstallPath: "../frosh/tools"},
		{Name: "unrelated/library", Type: "library", InstallPath: "../unrelated/library"},
	})

	args, plugins, err := requirePackages(composerJsonPath, "6.6.4.0")
	require.NoError(t, err)

	// First-party packages present in require are pinned to the target.
	assert.Contains(t, args, "shopware/core:6.6.4.0")
	assert.Contains(t, args, "shopware/storefront:6.6.4.0")
	// administration/elasticsearch are not required, so they are not pinned.
	assert.NotContains(t, args, "shopware/administration:6.6.4.0")

	// Only shopware-platform-plugins from require are listed, without a constraint.
	assert.Equal(t, []string{"frosh/tools", "swag/paypal"}, plugins)
	assert.Contains(t, args, "swag/paypal")
	assert.Contains(t, args, "frosh/tools")
	assert.NotContains(t, args, "unrelated/library")
}

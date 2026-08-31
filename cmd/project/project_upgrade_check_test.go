package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestGetLocalExtensionsReadsComposerLockAndCustomPlugins(t *testing.T) {
	p := testhelper.NewProject(t)
	t.Setenv("PROJECT_ROOT", p.Root)
	p.File("composer.lock", testhelper.ComposerLock(
		testhelper.LockPackage{Name: "shopware/core", Version: "v6.6.5.0"},
	))

	p.CustomPlugin("FroshTest", testhelper.PluginComposer("frosh/frosh-test", "1.2.0", `FroshTest\FroshTest`))
	// A plugin without a parseable version falls back to 1.0.0.
	p.CustomPlugin("NoVersion", testhelper.PluginComposer("frosh/no-version", "", `NoVersion\NoVersion`))

	coreVersion, extensions, err := getLocalExtensions()
	require.NoError(t, err)
	// The v prefix must be trimmed or the store compatibility API gets fed
	// a version it does not know.
	assert.Equal(t, "6.6.5.0", coreVersion.String())
	assert.Equal(t, "1.2.0", extensions["FroshTest"])
	assert.Equal(t, "1.0.0", extensions["NoVersion"])
}

func TestGetLocalExtensionsErrorsWhenCoreMissing(t *testing.T) {
	t.Run("missing composer.lock", func(t *testing.T) {
		t.Setenv("PROJECT_ROOT", t.TempDir())
		_, _, err := getLocalExtensions()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read composer.lock")
	})

	t.Run("core not in the lock file", func(t *testing.T) {
		p := testhelper.NewProject(t)
		t.Setenv("PROJECT_ROOT", p.Root)
		p.File("composer.lock", testhelper.ComposerLock(
			testhelper.LockPackage{Name: "vendor/other", Version: "1.0.0"},
		))

		_, _, err := getLocalExtensions()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "shopware/core package not found in composer.lock")
	})
}

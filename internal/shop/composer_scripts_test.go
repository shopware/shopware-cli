package shop

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/testhelper"
)

func TestGetComposerScripts(t *testing.T) {
	t.Run("missing composer.json", func(t *testing.T) {
		scripts, err := GetComposerScripts(t.TempDir())
		require.Error(t, err)
		assert.Nil(t, scripts)
	})

	t.Run("filters events and returns custom scripts", func(t *testing.T) {
		dir := t.TempDir()
		writeComposerJSON(t, dir, `{
			"scripts": {
				"auto-scripts": {"cache:clear": "symfony-cmd"},
				"post-install-cmd": ["@auto-scripts"],
				"post-update-cmd": ["@auto-scripts"],
				"ecs": "php vendor/bin/ecs check",
				"phpstan": "php vendor/bin/phpstan analyse",
				"setup": ["@composer install"]
			},
			"scripts-descriptions": {
				"phpstan": "Run PHPStan",
				"post-install-cmd": "ignored"
			},
			"scripts-aliases": {
				"phpstan": ["stan"]
			}
		}`)

		scripts, err := GetComposerScripts(dir)
		require.NoError(t, err)

		assert.Equal(t, []ComposerScript{
			{Name: "ecs"},
			{Name: "phpstan", Description: "Run PHPStan", Aliases: []string{"stan"}},
			{Name: "setup"},
		}, scripts)
	})

	t.Run("empty scripts section", func(t *testing.T) {
		dir := t.TempDir()
		writeComposerJSON(t, dir, `{"name": "acme/shop"}`)

		scripts, err := GetComposerScripts(dir)
		require.NoError(t, err)
		assert.Empty(t, scripts)
	})
}

func TestFindComposerScript(t *testing.T) {
	scripts := []ComposerScript{
		{Name: "phpstan", Aliases: []string{"stan"}},
		{Name: "stan"},
	}

	found, ok := FindComposerScript(scripts, "stan")
	require.True(t, ok)
	assert.Equal(t, "stan", found.Name)

	found, ok = FindComposerScript(scripts, "phpstan")
	require.True(t, ok)
	assert.Equal(t, "phpstan", found.Name)

	_, ok = FindComposerScript(scripts, "missing")
	assert.False(t, ok)
}

func writeComposerJSON(t *testing.T, dir, contents string) {
	t.Helper()
	testhelper.WriteFile(t, filepath.Join(dir, "composer.json"), contents)
}

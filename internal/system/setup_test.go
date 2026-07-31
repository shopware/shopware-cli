package system

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckProjectCreationDependenciesAllowsMissingComposer(t *testing.T) {
	php := filepath.Join(t.TempDir(), "php")
	require.NoError(t, os.WriteFile(php, []byte("#!/bin/sh\necho 'PHP 8.3.0 (cli)'\n"), 0o700))
	t.Setenv("PHP_BINARY", php)
	t.Setenv("PATH", t.TempDir())

	assert.Empty(t, CheckProjectCreationDependencies(t.Context(), false, nil))
	assert.Equal(t, []MissingDependency{{Name: "Composer", Reason: "not installed"}}, CheckProjectDependencies(t.Context(), false, nil))
}

func TestCheckIncompatibilities(t *testing.T) {
	t.Run("no incompatibilities on non-darwin", func(t *testing.T) {
		t.Setenv("HOME", "/tmp/test-home")
		incompatibilities := CheckIncompatibilities(false, "/tmp/project")
		assert.Empty(t, incompatibilities)
	})
}

func TestRenderMissingDependencies(t *testing.T) {
	t.Run("docker not running shows start message", func(t *testing.T) {
		out := RenderMissingDependencies(true, []MissingDependency{
			{Name: "Docker", Reason: "not running"},
		}, "create a Shopware project", "Then re-run with --docker")
		assert.Contains(t, out, "Start Docker and try again.")
		assert.NotContains(t, out, "install one of")
	})

	t.Run("docker not installed shows install message", func(t *testing.T) {
		out := RenderMissingDependencies(true, []MissingDependency{
			{Name: "Docker", Reason: "not installed"},
		}, "create a Shopware project", "Then re-run with --docker")
		assert.Contains(t, out, "Install Docker and try again.")
		assert.NotContains(t, out, "install one of")
	})

	t.Run("missing php and composer shows install links", func(t *testing.T) {
		out := RenderMissingDependencies(false, []MissingDependency{
			{Name: "PHP 8.2+", Reason: "not installed"},
			{Name: "Composer", Reason: "not installed"},
		}, "create a Shopware project", "re-run with --docker")
		assert.Contains(t, out, "To create a Shopware project, either:")
		assert.Contains(t, out, "Docker")
		assert.Contains(t, out, "(recommended)")
		assert.Contains(t, out, "re-run with --docker")
		assert.Contains(t, out, "Install PHP 8.2+ and Composer, or point PHP_BINARY at a matching PHP binary")
		assert.Contains(t, out, "PHP_BINARY=/usr/bin/php8.2")
		assert.Contains(t, out, "https://www.php.net/downloads.php")
		assert.Contains(t, out, "https://getcomposer.org/")
	})

	t.Run("php constraint mismatch mentions PHP_BINARY", func(t *testing.T) {
		out := RenderMissingDependencies(false, []MissingDependency{
			{Name: "PHP ~8.2.0 || ~8.3.0", Reason: "found PHP 8.4.22"},
		}, "create a Shopware project", "re-run with --docker")
		assert.Contains(t, out, "To create a Shopware project, either:")
		assert.Contains(t, out, "Docker")
		assert.Contains(t, out, "(recommended)")
		assert.Contains(t, out, "re-run with --docker")
		assert.Contains(t, out, "Install a PHP version matching ~8.2.0 || ~8.3.0, or point PHP_BINARY at one")
		assert.Contains(t, out, "PHP_BINARY=/usr/bin/php8.3")
		assert.Contains(t, out, "https://www.php.net/downloads.php")
		assert.NotContains(t, out, "Composer")
	})

	t.Run("action phrases the help text", func(t *testing.T) {
		out := RenderMissingDependencies(false, []MissingDependency{
			{Name: "PHP 8.2+", Reason: "not installed"},
		}, "start the development environment", "")
		assert.Contains(t, out, "To start the development environment, either:")
		assert.Contains(t, out, "re-run with")
		assert.Contains(t, out, "--docker")
	})
}

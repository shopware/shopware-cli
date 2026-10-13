package system

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func stubDocker(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\n"+script+"\n"), 0o755))
	t.Setenv("PATH", dir)
}

func TestCheckProjectDependencies(t *testing.T) {
	if runtime.GOOS == "windows" || IsInsideContainer() {
		t.Skip("needs a shell script as docker stub outside a container")
	}

	t.Run("docker not installed", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		assert.Equal(t, []MissingDependency{{Name: "Docker", Reason: "not installed"}}, CheckProjectDependencies(t.Context(), true, nil, ""))
	})

	t.Run("compose plugin missing", func(t *testing.T) {
		stubDocker(t, `[ "$1" = compose ] && exit 1; exit 0`)
		assert.Equal(t, []MissingDependency{{Name: "Docker Compose plugin", Reason: "not installed"}}, CheckProjectDependencies(t.Context(), true, nil, ""))
	})

	t.Run("daemon down and compose plugin missing", func(t *testing.T) {
		stubDocker(t, `exit 1`)
		assert.Equal(t, []MissingDependency{{Name: "Docker", Reason: "not running"}, {Name: "Docker Compose plugin", Reason: "not installed"}}, CheckProjectDependencies(t.Context(), true, nil, ""))
	})

	t.Run("docker and compose plugin available", func(t *testing.T) {
		stubDocker(t, `exit 0`)
		assert.Empty(t, CheckProjectDependencies(t.Context(), true, nil, ""))
	})
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

	t.Run("compose plugin missing skips the php alternative", func(t *testing.T) {
		out := RenderMissingDependencies(true, []MissingDependency{
			{Name: "Docker Compose plugin", Reason: "not installed"},
		}, "start the development environment", "")
		assert.NotContains(t, out, "either:")
		assert.NotContains(t, out, "php.net")
	})

	t.Run("missing php shows install links", func(t *testing.T) {
		out := RenderMissingDependencies(false, []MissingDependency{
			{Name: "PHP 8.2+", Reason: "not installed"},
		}, "create a Shopware project", "re-run with --docker")
		assert.Contains(t, out, "To create a Shopware project, either:")
		assert.Contains(t, out, "Docker")
		assert.Contains(t, out, "(recommended)")
		assert.Contains(t, out, "re-run with --docker")
		assert.Contains(t, out, "Install a PHP version matching 8.2+, or point PHP_BINARY at one")
		assert.Contains(t, out, "PHP_BINARY=/usr/bin/php8.2")
		assert.Contains(t, out, "https://www.php.net/downloads.php")
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

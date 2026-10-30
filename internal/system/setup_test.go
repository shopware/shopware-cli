package system

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
		assert.Equal(t, []MissingDependency{{Name: "Docker", Reason: "not installed", ID: "docker_not_installed"}}, CheckProjectDependencies(t.Context(), true, nil, ""))
	})

	t.Run("compose plugin missing", func(t *testing.T) {
		stubDocker(t, `[ "$1" = compose ] && exit 1; exit 0`)
		assert.Equal(t, []MissingDependency{{Name: "Docker Compose plugin", Reason: "not installed", ID: "compose_plugin_missing"}}, CheckProjectDependencies(t.Context(), true, nil, ""))
	})

	t.Run("daemon down and compose plugin missing", func(t *testing.T) {
		stubDocker(t, `exit 1`)
		assert.Equal(t, []MissingDependency{{Name: "Docker", Reason: "not running", ID: "docker_not_running"}, {Name: "Docker Compose plugin", Reason: "not installed", ID: "compose_plugin_missing"}}, CheckProjectDependencies(t.Context(), true, nil, ""))
	})

	t.Run("compose plugin is present when the daemon is down", func(t *testing.T) {
		stubDocker(t, `if [ "$1" = version ]; then exit 1; fi; exit 0`)
		assert.Equal(t, []MissingDependency{{Name: "Docker", Reason: "not running", ID: "docker_not_running"}}, CheckProjectDependencies(t.Context(), true, nil, ""))
	})

	t.Run("slow daemon response is running", func(t *testing.T) {
		// The stub's PATH contains only the fake docker, so sleep must be absolute.
		stubDocker(t, `
if [ "$1" = version ]; then
  /bin/sleep 1
  exit 0
fi
exit 0
`)
		assert.Empty(t, CheckProjectDependencies(t.Context(), true, nil, ""))
	})

	t.Run("timed out daemon probe still checks compose", func(t *testing.T) {
		previous := dockerProbeTimeout
		dockerProbeTimeout = 200 * time.Millisecond
		t.Cleanup(func() { dockerProbeTimeout = previous })

		stubDocker(t, `
if [ "$1" = version ]; then
  exec /bin/sleep 30
fi
exit 0
`)
		started := time.Now()
		missing := CheckProjectDependencies(t.Context(), true, nil, "")
		assert.Less(t, time.Since(started), 5*time.Second)
		assert.Equal(t, []MissingDependency{{Name: "Docker", Reason: "not running", ID: "docker_not_running"}}, missing)
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
			{Name: "Docker", Reason: "not running", ID: "docker_not_running"},
		}, "create a Shopware project", "Then re-run with --docker")
		assert.Contains(t, out, "Start Docker and try again.")
		assert.NotContains(t, out, "install one of")
	})

	t.Run("docker not installed shows install message", func(t *testing.T) {
		out := RenderMissingDependencies(true, []MissingDependency{
			{Name: "Docker", Reason: "not installed", ID: "docker_not_installed"},
		}, "create a Shopware project", "Then re-run with --docker")
		assert.Contains(t, out, "Install Docker and try again.")
		assert.NotContains(t, out, "install one of")
	})

	t.Run("compose plugin missing skips the php alternative", func(t *testing.T) {
		out := RenderMissingDependencies(true, []MissingDependency{
			{Name: "Docker Compose plugin", Reason: "not installed", ID: "compose_plugin_missing"},
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

func TestCheckProjectDependenciesReportsPHPMissingWithID(t *testing.T) {
	t.Parallel()

	missing := CheckProjectDependencies(t.Context(), false, nil, filepath.Join(t.TempDir(), "php"))

	assert.Equal(t, []string{"php_not_installed"}, (&MissingDependenciesError{Missing: missing}).IDs())
}

func TestMissingDependenciesErrorIDsFallBackToOther(t *testing.T) {
	t.Parallel()

	err := &MissingDependenciesError{Missing: []MissingDependency{{Name: "Something", Reason: "gone"}}}

	assert.Equal(t, []string{"other"}, err.IDs())
}

func TestMissingDependenciesErrorIDsAreSortedAndUnique(t *testing.T) {
	t.Parallel()

	err := &MissingDependenciesError{Missing: []MissingDependency{
		{Name: "Docker Compose plugin", Reason: "not installed", ID: "compose_plugin_missing"},
		{Name: "Docker", Reason: "not running", ID: "docker_not_running"},
		{Name: "Docker", Reason: "not running", ID: "docker_not_running"},
	}}

	assert.Equal(t, []string{"compose_plugin_missing", "docker_not_running"}, err.IDs())

	var target *MissingDependenciesError
	assert.ErrorAs(t, fmt.Errorf("cannot create project: %w", err), &target)
}

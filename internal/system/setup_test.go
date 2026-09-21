package system

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/oci"
	"github.com/shopware/shopware-cli/internal/oci/ocitest"
)

func TestCheckProjectDependenciesDocker(t *testing.T) {
	if IsInsideContainer() {
		t.Skip("the docker dependency check is bypassed inside containers")
	}

	t.Run("runtime installed and running", func(t *testing.T) {
		ctx := oci.WithRuntime(t.Context(), &ocitest.Runtime{AvailableV: true})
		assert.Empty(t, CheckProjectDependencies(ctx, true, nil, ""))
	})

	t.Run("runtime not running", func(t *testing.T) {
		rt := &ocitest.Runtime{AvailableV: true, NewCmd: func(c *ocitest.Cmd) {
			c.RunFunc = func(*ocitest.Cmd) error { return errors.New("cannot connect to the daemon") }
		}}
		ctx := oci.WithRuntime(t.Context(), rt)
		missing := CheckProjectDependencies(ctx, true, nil, "")
		require.Len(t, missing, 1)
		assert.Equal(t, MissingDependency{Name: "Docker", Reason: "not running"}, missing[0])
	})

	t.Run("runtime not installed", func(t *testing.T) {
		ctx := oci.WithRuntime(t.Context(), &ocitest.Runtime{AvailableV: false})
		missing := CheckProjectDependencies(ctx, true, nil, "")
		require.Len(t, missing, 1)
		assert.Equal(t, MissingDependency{Name: "Docker", Reason: "not installed"}, missing[0])
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

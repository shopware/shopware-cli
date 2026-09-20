package system

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/oci"
)

// stubRuntime is an oci.Runtime whose binary is a stub shell script.
type stubRuntime struct{ binary string }

func (s stubRuntime) Binary() string { return s.binary }

func (s stubRuntime) Command(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, s.binary, args...)
}

func (s stubRuntime) ComposeCommand(ctx context.Context, args ...string) *exec.Cmd {
	return s.Command(ctx, append([]string{"compose"}, args...)...)
}

// writeStubRuntime writes a stub runtime binary running script and returns it
// as an oci.Runtime.
func writeStubRuntime(t *testing.T, script string) oci.Runtime {
	t.Helper()

	if runtime.GOOS == "windows" {
		t.Skip("stub runtime binary requires a POSIX shell")
	}

	shPath, err := exec.LookPath("sh")
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "oci-stub")
	require.NoError(t, os.WriteFile(path, []byte("#!"+shPath+"\n"+script), 0o755))

	return stubRuntime{binary: path}
}

func TestCheckProjectDependenciesDocker(t *testing.T) {
	if IsInsideContainer() {
		t.Skip("the docker dependency check is bypassed inside containers")
	}

	t.Run("runtime installed and running", func(t *testing.T) {
		ctx := oci.WithRuntime(t.Context(), writeStubRuntime(t, "exit 0"))
		assert.Empty(t, CheckProjectDependencies(ctx, true, nil, ""))
	})

	t.Run("runtime not running", func(t *testing.T) {
		ctx := oci.WithRuntime(t.Context(), writeStubRuntime(t, "exit 1"))
		missing := CheckProjectDependencies(ctx, true, nil, "")
		require.Len(t, missing, 1)
		assert.Equal(t, MissingDependency{Name: "Docker", Reason: "not running"}, missing[0])
	})

	t.Run("runtime not installed", func(t *testing.T) {
		ctx := oci.WithRuntime(t.Context(), stubRuntime{binary: filepath.Join(t.TempDir(), "does-not-exist")})
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

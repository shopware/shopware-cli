package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/system"
)

func TestNewLocalExecutor(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")

	cfg := &shop.EnvironmentConfig{Type: "local"}

	exec, err := New("/project", cfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, "local", exec.Type())
}

func TestNewLocalExecutorEmptyType(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")

	cfg := &shop.EnvironmentConfig{Type: ""}

	exec, err := New("/project", cfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, "local", exec.Type())
}

func TestNewDockerExecutor(t *testing.T) {
	cfg := &shop.EnvironmentConfig{Type: "docker"}

	exec, err := New("/project", cfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, "docker", exec.Type())
}

func TestNewUnsupportedType(t *testing.T) {
	cfg := &shop.EnvironmentConfig{Type: "unknown"}

	_, err := New("/project", cfg, &shop.Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported environment type: unknown")
}

func TestLocalExecutorConsoleCommand(t *testing.T) {
	t.Setenv("PHP_BINARY", "")
	exec := &LocalExecutor{projectRoot: "/project"}

	p := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, []string{"php", "bin/console", "cache:clear"}, p.Cmd.Args)
	assert.Equal(t, "/project", p.Cmd.Dir)
}

func TestLocalExecutorComposerCommand(t *testing.T) {
	t.Setenv("PHP_BINARY", "")
	stubComposer(t, "/usr/local/bin/composer", false, nil)
	exec := &LocalExecutor{projectRoot: "/project"}

	p := exec.ComposerCommand(t.Context(), "install")
	assert.Equal(t, []string{"composer", "install"}, p.Cmd.Args)
	assert.Equal(t, "/project", p.Cmd.Dir)
}

func TestLocalExecutorPHPCommand(t *testing.T) {
	t.Setenv("PHP_BINARY", "")
	exec := &LocalExecutor{projectRoot: "/project"}

	p := exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, []string{"php", "-v"}, p.Cmd.Args)
	assert.Equal(t, "/project", p.Cmd.Dir)
}

// writeFakePHPBinary creates an executable reporting the given PHP version.
func writeFakePHPBinary(t *testing.T, path string, version string) {
	t.Helper()
	shPath, err := osexec.LookPath("sh")
	assert.NoError(t, err)

	script := fmt.Sprintf("#!%s\necho PHP %s\n", shPath, version)
	assert.NoError(t, os.WriteFile(path, []byte(script), 0o755))
}

// stubComposer resolves Composer to the given path, so tests neither depend on
// a Composer installation nor trigger a real PHAR download.
func stubComposer(t *testing.T, path string, isPhar bool, err error) {
	t.Helper()
	original := resolveComposer
	resolveComposer = func(context.Context) (string, bool, error) {
		return path, isPhar, err
	}
	t.Cleanup(func() { resolveComposer = original })
}

// stubPinnedPHP resolves a php_version to the given binary, keeping PHP_BINARY's
// real precedence over the pin.
func stubPinnedPHP(t *testing.T, binary string, err error) {
	t.Helper()
	original := resolveProjectPHPBinary
	resolveProjectPHPBinary = func(_ context.Context, pin string) (string, error) {
		if env := os.Getenv("PHP_BINARY"); env != "" {
			return env, nil
		}
		if pin == "" {
			return "", nil
		}
		return binary, err
	}
	t.Cleanup(func() { resolveProjectPHPBinary = original })
}

func TestLocalExecutorUsesProjectPHPVersion(t *testing.T) {
	t.Setenv("PHP_BINARY", "")
	stubPinnedPHP(t, "/opt/homebrew/opt/php@8.3/bin/php", nil)
	exec := &LocalExecutor{projectRoot: "/project", shopCfg: &shop.Config{PHPVersion: "8.3"}}

	p := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, []string{"/opt/homebrew/opt/php@8.3/bin/php", "bin/console", "cache:clear"}, p.Cmd.Args)

	p = exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, []string{"/opt/homebrew/opt/php@8.3/bin/php", "-v"}, p.Cmd.Args)
}

func TestLocalExecutorPHPBinaryEnvOverridesProjectPHPVersion(t *testing.T) {
	t.Setenv("PHP_BINARY", "/env/php")
	stubPinnedPHP(t, "/opt/homebrew/opt/php@8.3/bin/php", nil)
	exec := &LocalExecutor{projectRoot: "/project", shopCfg: &shop.Config{PHPVersion: "8.3"}}

	p := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, []string{"/env/php", "bin/console", "cache:clear"}, p.Cmd.Args)

	p = exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, []string{"/env/php", "-v"}, p.Cmd.Args)
}

func TestLocalExecutorReportsUnresolvablePHPVersion(t *testing.T) {
	t.Setenv("PHP_BINARY", "")
	notFound := &system.PHPVersionNotFoundError{Pin: "8.3"}
	stubPinnedPHP(t, "", notFound)
	exec := &LocalExecutor{projectRoot: "/project", shopCfg: &shop.Config{PHPVersion: "8.3"}}

	// The error is carried by the command so it surfaces when it runs.
	for name, p := range map[string]*Process{
		"console":  exec.ConsoleCommand(t.Context(), "cache:clear"),
		"php":      exec.PHPCommand(t.Context(), "-v"),
		"composer": exec.ComposerCommand(t.Context(), "install"),
	} {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, p.Cmd.Err, notFound)
			assert.ErrorIs(t, p.Run(), notFound)
		})
	}
}

func TestLocalExecutorFallsBackToPHPBinaryEnv(t *testing.T) {
	dir := t.TempDir()
	writeFakePHPBinary(t, filepath.Join(dir, "php"), "8.3.19")
	t.Setenv("PHP_BINARY", filepath.Join(dir, "php"))
	exec := &LocalExecutor{projectRoot: "/project", shopCfg: &shop.Config{}}

	p := exec.PHPCommand(t.Context(), "-v")
	resolved, err := filepath.EvalSymlinks(filepath.Join(dir, "php"))
	assert.NoError(t, err)
	assert.Equal(t, []string{resolved, "-v"}, p.Cmd.Args)
}

func TestLocalExecutorRejectsUnusablePHPBinaryEnv(t *testing.T) {
	t.Setenv("PHP_BINARY", "/does/not/exist/php")
	exec := &LocalExecutor{projectRoot: "/project", shopCfg: &shop.Config{}}

	p := exec.PHPCommand(t.Context(), "-v")
	assert.ErrorContains(t, p.Cmd.Err, "PHP_BINARY is set but unusable")
	assert.ErrorContains(t, p.Run(), "PHP_BINARY is set but unusable")
}

func TestLocalExecutorComposerRunsThroughSelectedPHP(t *testing.T) {
	binDir := t.TempDir()
	composerPath := filepath.Join(binDir, "composer")
	assert.NoError(t, os.WriteFile(composerPath, []byte("#!/bin/sh\n"), 0o755))
	t.Setenv("PATH", binDir)
	t.Setenv("PHP_BINARY", "")

	stubPinnedPHP(t, "/custom/php", nil)
	exec := &LocalExecutor{projectRoot: "/project", shopCfg: &shop.Config{PHPVersion: "8.3"}}

	p := exec.ComposerCommand(t.Context(), "install")
	assert.Equal(t, []string{"/custom/php", composerPath, "install"}, p.Cmd.Args)
}

func TestLocalExecutorComposerUsesDownloadedPharWithoutComposerInPath(t *testing.T) {
	t.Setenv("PHP_BINARY", "")
	stubComposer(t, "/cache/shopware-cli/composer.phar", true, nil)

	t.Run("with pinned php", func(t *testing.T) {
		stubPinnedPHP(t, "/custom/php", nil)
		exec := &LocalExecutor{projectRoot: "/project", shopCfg: &shop.Config{PHPVersion: "8.3"}}

		p := exec.ComposerCommand(t.Context(), "install")
		assert.Equal(t, []string{"/custom/php", "/cache/shopware-cli/composer.phar", "install"}, p.Cmd.Args)
	})

	t.Run("with default php", func(t *testing.T) {
		exec := &LocalExecutor{projectRoot: "/project"}

		p := exec.ComposerCommand(t.Context(), "install")
		assert.Equal(t, []string{"php", "/cache/shopware-cli/composer.phar", "install"}, p.Cmd.Args)
	})
}

func TestLocalExecutorComposerReportsFailedDownload(t *testing.T) {
	t.Setenv("PHP_BINARY", "")
	downloadErr := errors.New("cannot download composer: connection refused")
	stubComposer(t, "", false, downloadErr)
	exec := &LocalExecutor{projectRoot: "/project"}

	p := exec.ComposerCommand(t.Context(), "install")
	assert.ErrorIs(t, p.Cmd.Err, downloadErr)
	assert.ErrorIs(t, p.Run(), downloadErr)
}

func TestSymfonyCLIExecutorConsoleCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}

	p := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, []string{"/usr/local/bin/symfony", "php", "bin/console", "cache:clear"}, p.Cmd.Args)
	assert.Equal(t, "/project", p.Cmd.Dir)
}

func TestSymfonyCLIExecutorComposerCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}

	p := exec.ComposerCommand(t.Context(), "install")
	assert.Equal(t, []string{"/usr/local/bin/symfony", "composer", "install"}, p.Cmd.Args)
}

func TestSymfonyCLIExecutorPHPCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}

	p := exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, []string{"/usr/local/bin/symfony", "php", "-v"}, p.Cmd.Args)
}

func TestDockerExecutorConsoleCommand(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	p := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Contains(t, p.Cmd.Path, "docker")
	assert.Contains(t, p.Cmd.Args, "compose")
	assert.Contains(t, p.Cmd.Args, "exec")
	assert.Contains(t, p.Cmd.Args, "web")
	assert.Contains(t, p.Cmd.Args, "php")
	assert.Contains(t, p.Cmd.Args, "bin/console")
	assert.Contains(t, p.Cmd.Args, "cache:clear")
	assert.Equal(t, "/project", p.Cmd.Dir)
	assert.Contains(t, p.Cmd.Args, "--workdir")
	assert.Contains(t, p.Cmd.Args, "/var/www/html")
}

func TestDockerExecutorComposerCommand(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	p := exec.ComposerCommand(t.Context(), "install", "--no-interaction")
	assert.Contains(t, p.Cmd.Path, "docker")
	assert.Contains(t, p.Cmd.Args, "compose")
	assert.Contains(t, p.Cmd.Args, "exec")
	assert.Contains(t, p.Cmd.Args, "web")
	assert.Contains(t, p.Cmd.Args, "composer")
	assert.Contains(t, p.Cmd.Args, "install")
	assert.Contains(t, p.Cmd.Args, "--no-interaction")
}

func TestDockerExecutorPHPCommand(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	p := exec.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Path, "docker")
	assert.Contains(t, p.Cmd.Args, "compose")
	assert.Contains(t, p.Cmd.Args, "exec")
	assert.Contains(t, p.Cmd.Args, "web")
	assert.Contains(t, p.Cmd.Args, "php")
	assert.Contains(t, p.Cmd.Args, "-v")
}

// composeProjectArgIndex returns the index of the "-p" flag in args, or -1.
func composeProjectArgIndex(args []string) int {
	for i, arg := range args {
		if arg == "-p" {
			return i
		}
	}
	return -1
}

func TestDockerExecutorPinsComposeProjectName(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project", composeProjectName: "sw-shop-abc123"}

	for _, p := range []*Process{
		exec.ConsoleCommand(t.Context(), "cache:clear"),
		exec.ComposerCommand(t.Context(), "install"),
		exec.PHPCommand(t.Context(), "-v"),
		exec.NPMCommand(t.Context(), "run", "dev"),
	} {
		i := composeProjectArgIndex(p.Cmd.Args)
		require.Greater(t, i, 0, "compose invocation carries -p: %v", p.Cmd.Args)
		assert.Equal(t, "compose", p.Cmd.Args[i-1], "-p directly follows compose")
		assert.Equal(t, "sw-shop-abc123", p.Cmd.Args[i+1])
	}

	// The pin survives the executor's copy-on-write helpers.
	for _, derived := range []Executor{
		exec.WithEnv(map[string]string{"FOO": "bar"}),
		exec.WithRelDir("custom/plugins"),
	} {
		p := derived.PHPCommand(t.Context(), "-v")
		i := composeProjectArgIndex(p.Cmd.Args)
		require.Greater(t, i, 0)
		assert.Equal(t, "sw-shop-abc123", p.Cmd.Args[i+1])
	}
}

func TestDockerExecutorWithoutComposeProjectName(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	p := exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, -1, composeProjectArgIndex(p.Cmd.Args), "no -p flag without a configured project name")
}

func TestNewDockerExecutorReadsComposeProjectName(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("COMPOSE_PROJECT_NAME=sw-shop-abc123\n"), 0o644))

	t.Run("snapshots the .env value at construction", func(t *testing.T) {
		t.Setenv(shop.ComposeProjectNameEnvKey, "")
		exec, err := New(dir, &shop.EnvironmentConfig{Type: "docker"}, &shop.Config{})
		require.NoError(t, err)

		// A later .env rewrite (e.g. a Flex recipe reset mid-upgrade) must not
		// detach the executor from the containers it started with.
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("APP_ENV=prod\n"), 0o644))

		p := exec.PHPCommand(t.Context(), "-v")
		i := composeProjectArgIndex(p.Cmd.Args)
		require.Greater(t, i, 0)
		assert.Equal(t, "sw-shop-abc123", p.Cmd.Args[i+1])
	})

	t.Run("a process-level COMPOSE_PROJECT_NAME stays authoritative", func(t *testing.T) {
		t.Setenv(shop.ComposeProjectNameEnvKey, "from-shell")
		exec, err := New(dir, &shop.EnvironmentConfig{Type: "docker"}, &shop.Config{})
		require.NoError(t, err)

		p := exec.PHPCommand(t.Context(), "-v")
		assert.Equal(t, -1, composeProjectArgIndex(p.Cmd.Args),
			"the inherited environment variable already outranks .env for every docker invocation")
	})
}

func TestConsoleCommandNameDefault(t *testing.T) {
	assert.Equal(t, "bin/console", consoleCommandName(t.Context()))
}

func TestConsoleCommandNameWithAllowBinCI(t *testing.T) {
	t.Setenv("CI", "true")

	ctx := AllowBinCI(t.Context())
	assert.Equal(t, "bin/ci", consoleCommandName(ctx))
}

func TestLocalExecutorWithEnv(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	withEnv := exec.WithEnv(map[string]string{
		"INSTALL_LOCALE":   "de-DE",
		"INSTALL_CURRENCY": "EUR",
	})

	p := withEnv.PHPCommand(t.Context(), "vendor/bin/shopware-deployment-helper", "run")
	assert.Contains(t, p.Cmd.Env, "INSTALL_LOCALE=de-DE")
	assert.Contains(t, p.Cmd.Env, "INSTALL_CURRENCY=EUR")
}

func TestLocalExecutorWithoutEnv(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}

	p := exec.PHPCommand(t.Context(), "-v")
	assert.NotNil(t, p.Cmd.Env)
	assert.Contains(t, p.Cmd.Env, "PROJECT_ROOT=/project")
}

func TestDockerExecutorWithEnv(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}
	withEnv := exec.WithEnv(map[string]string{
		"INSTALL_LOCALE": "en-GB",
	})

	p := withEnv.PHPCommand(t.Context(), "vendor/bin/shopware-deployment-helper", "run")
	assert.Contains(t, p.Cmd.Args, "-e")
	assert.Contains(t, p.Cmd.Args, "INSTALL_LOCALE=en-GB")
}

func TestSymfonyCLIExecutorWithEnv(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}
	withEnv := exec.WithEnv(map[string]string{
		"INSTALL_LOCALE": "de-DE",
	})

	p := withEnv.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Env, "INSTALL_LOCALE=de-DE")
}

func TestLocalExecutorNPMCommand(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}

	p := exec.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, []string{"npm", "run", "dev"}, p.Cmd.Args)
	assert.Equal(t, "/project", p.Cmd.Dir)
}

func TestDockerExecutorNPMCommand(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	p := exec.NPMCommand(t.Context(), "run", "dev")
	assert.Contains(t, p.Cmd.Args, "compose")
	assert.Contains(t, p.Cmd.Args, "exec")
	assert.Contains(t, p.Cmd.Args, "web")
	assert.Contains(t, p.Cmd.Args, "npm")
	assert.Contains(t, p.Cmd.Args, "run")
	assert.Contains(t, p.Cmd.Args, "dev")
}

func TestDockerExecutorSetsHomeForMappedUser(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the user mapping and HOME redirect are Linux-only")
	}

	exec := &DockerExecutor{projectRoot: "/project"}

	// Every container command must carry HOME=/tmp so that npm (install,
	// run, exec) and composer do not fall back to / when the container runs
	// as the mapped host UID with no passwd entry.
	for _, p := range []*Process{
		exec.NPMCommand(t.Context(), "install"),
		exec.NPMCommand(t.Context(), "run", "build"),
		exec.ComposerCommand(t.Context(), "install"),
		exec.ConsoleCommand(t.Context(), "cache:clear"),
	} {
		assert.Contains(t, p.Cmd.Args, "HOME=/tmp")
	}
}

func TestSymfonyCLIExecutorNPMCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}

	p := exec.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, []string{"npm", "run", "dev"}, p.Cmd.Args)
}

func TestLocalExecutorWithRelDir(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	withDir := exec.WithRelDir("vendor/shopware/administration/Resources/app/administration")

	p := withDir.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, "/project/vendor/shopware/administration/Resources/app/administration", p.Cmd.Dir)

	p = withDir.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, "/project/vendor/shopware/administration/Resources/app/administration", p.Cmd.Dir)
}

func TestDockerExecutorWithRelDir(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	p := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, "/project", p.Cmd.Dir)
	assert.Contains(t, p.Cmd.Args, "--workdir")
	assert.Contains(t, p.Cmd.Args, "/var/www/html")

	withDir := exec.WithRelDir("vendor/shopware/administration/Resources/app/administration")

	p = withDir.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, "/project", p.Cmd.Dir)
	assert.Contains(t, p.Cmd.Args, "--workdir")
	assert.Contains(t, p.Cmd.Args, "/var/www/html/vendor/shopware/administration/Resources/app/administration")
}

func TestSymfonyCLIExecutorWithRelDir(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}
	withDir := exec.WithRelDir("vendor/shopware/administration/Resources/app/administration")

	p := withDir.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, "/project/vendor/shopware/administration/Resources/app/administration", p.Cmd.Dir)

	p = withDir.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, "/project/vendor/shopware/administration/Resources/app/administration", p.Cmd.Dir)
}

func TestWithRelDirPreservesEnv(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	withEnv := exec.WithEnv(map[string]string{"FOO": "bar"})
	withDirAndEnv := withEnv.WithRelDir("subdir")

	p := withDirAndEnv.PHPCommand(t.Context(), "-v")
	assert.Equal(t, "/project/subdir", p.Cmd.Dir)
	assert.Contains(t, p.Cmd.Env, "FOO=bar")
}

func TestWithEnvPreservesRelDir(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	withDir := exec.WithRelDir("subdir")
	withDirAndEnv := withDir.WithEnv(map[string]string{"FOO": "bar"})

	p := withDirAndEnv.PHPCommand(t.Context(), "-v")
	assert.Equal(t, "/project/subdir", p.Cmd.Dir)
	assert.Contains(t, p.Cmd.Env, "FOO=bar")
}

func TestWithEnvMerges(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	withA := exec.WithEnv(map[string]string{"A": "1"})
	withAB := withA.WithEnv(map[string]string{"B": "2"})

	p := withAB.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Env, "A=1")
	assert.Contains(t, p.Cmd.Env, "B=2")
}

func TestWithEnvOverrides(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	withA := exec.WithEnv(map[string]string{"A": "1"})
	withA2 := withA.WithEnv(map[string]string{"A": "2"})

	p := withA2.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Env, "A=2")
	assert.NotContains(t, p.Cmd.Env, "A=1")
}

func TestDockerWithEnvNormalizesProjectRoot(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/host/project"}
	withEnv := exec.WithEnv(map[string]string{
		"PROJECT_ROOT": "/host/project",
	})

	p := withEnv.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Args, "PROJECT_ROOT=/var/www/html")
}

func TestDockerWithEnvNormalizesAdminRoot(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/host/project"}
	withEnv := exec.WithEnv(map[string]string{
		"ADMIN_ROOT": "/host/project/vendor/shopware/administration/Resources/app/administration",
	})

	p := withEnv.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Args, "ADMIN_ROOT=/var/www/html/vendor/shopware/administration/Resources/app/administration")
}

func TestDockerWithEnvNormalizesStorefrontRoot(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/host/project"}
	withEnv := exec.WithEnv(map[string]string{
		"STOREFRONT_ROOT": "/host/project/vendor/shopware/storefront/Resources/app/storefront",
	})

	p := withEnv.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Args, "STOREFRONT_ROOT=/var/www/html/vendor/shopware/storefront/Resources/app/storefront")
}

func TestDockerWithEnvDoesNotNormalizeUnrelatedEnv(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/host/project"}
	withEnv := exec.WithEnv(map[string]string{
		"SOME_PATH": "/host/project/something",
	})

	p := withEnv.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Args, "SOME_PATH=/host/project/something")
}

func TestDockerWithEnvDoesNotNormalizeNonMatchingPrefix(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/host/project"}
	withEnv := exec.WithEnv(map[string]string{
		"PROJECT_ROOT": "/other/path",
	})

	p := withEnv.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Args, "PROJECT_ROOT=/other/path")
}

func TestDockerWithEnvMerges(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}
	withA := exec.WithEnv(map[string]string{"A": "1"})
	withAB := withA.WithEnv(map[string]string{"B": "2"})

	p := withAB.PHPCommand(t.Context(), "-v")
	assert.Contains(t, p.Cmd.Args, "A=1")
	assert.Contains(t, p.Cmd.Args, "B=2")
}

func TestNewLocal(t *testing.T) {
	exec := NewLocal("/my/project")

	p := exec.NPMCommand(t.Context(), "install")
	assert.Equal(t, "/my/project", p.Cmd.Dir)
	assert.Equal(t, []string{"npm", "install"}, p.Cmd.Args)
}

func TestNewLocalWithConfig(t *testing.T) {
	envCfg := &shop.EnvironmentConfig{Type: TypeDocker}
	shopCfg := &shop.Config{}

	exec := NewLocalWithConfig("/my/project", envCfg, shopCfg)
	localExec, ok := exec.(*LocalExecutor)

	assert.True(t, ok)
	assert.Equal(t, TypeLocal, exec.Type())
	assert.Equal(t, "/my/project", localExec.projectRoot)
	assert.Same(t, envCfg, localExec.envCfg)
	assert.Same(t, shopCfg, localExec.shopCfg)
}

func TestLocalNormalizePath(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/host/project"}
	assert.Equal(t, "/host/project/custom/plugins/MyPlugin", exec.NormalizePath("/host/project/custom/plugins/MyPlugin"))
}

func TestDockerNormalizePath(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/host/project"}
	assert.Equal(t, "/var/www/html/custom/plugins/MyPlugin", exec.NormalizePath("/host/project/custom/plugins/MyPlugin"))
	assert.Equal(t, "/var/www/html", exec.NormalizePath("/host/project"))
}

func TestSymfonyCLINormalizePath(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/host/project"}
	assert.Equal(t, "/host/project/custom/plugins/MyPlugin", exec.NormalizePath("/host/project/custom/plugins/MyPlugin"))
}

func TestLocalExecutorEnvironmentStatusNotSupported(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}

	running, err := exec.EnvironmentStatus(t.Context())
	assert.False(t, running)
	assert.ErrorIs(t, err, ErrNotSupported)
}

func TestSymfonyCLIExecutorEnvironmentStatusNotSupported(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}

	running, err := exec.EnvironmentStatus(t.Context())
	assert.False(t, running)
	assert.ErrorIs(t, err, ErrNotSupported)
}

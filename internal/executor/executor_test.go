package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
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
	exec := &LocalExecutor{projectRoot: "/project"}

	cmd := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, []string{"php", "bin/console", "cache:clear"}, cmd.Args)
	assert.Equal(t, "/project", cmd.Dir)
}

func TestLocalExecutorComposerCommand(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}

	cmd := exec.ComposerCommand(t.Context(), "install")
	assert.Equal(t, []string{"composer", "install"}, cmd.Args)
	assert.Equal(t, "/project", cmd.Dir)
}

func TestLocalExecutorPHPCommand(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}

	cmd := exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, []string{"php", "-v"}, cmd.Args)
	assert.Equal(t, "/project", cmd.Dir)
}

func TestSymfonyCLIExecutorConsoleCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}

	cmd := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, []string{"/usr/local/bin/symfony", "php", "bin/console", "cache:clear"}, cmd.Args)
	assert.Equal(t, "/project", cmd.Dir)
}

func TestSymfonyCLIExecutorComposerCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}

	cmd := exec.ComposerCommand(t.Context(), "install")
	assert.Equal(t, []string{"/usr/local/bin/symfony", "composer", "install"}, cmd.Args)
}

func TestSymfonyCLIExecutorPHPCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}

	cmd := exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, []string{"/usr/local/bin/symfony", "php", "-v"}, cmd.Args)
}

func TestDockerExecutorConsoleCommand(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	cmd := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Contains(t, cmd.Path, "docker")
	assert.Contains(t, cmd.Args, "compose")
	assert.Contains(t, cmd.Args, "exec")
	assert.Contains(t, cmd.Args, "web")
	assert.Contains(t, cmd.Args, "php")
	assert.Contains(t, cmd.Args, "bin/console")
	assert.Contains(t, cmd.Args, "cache:clear")
	assert.Equal(t, "/project", cmd.Dir)
	assert.Contains(t, cmd.Args, "--workdir")
	assert.Contains(t, cmd.Args, "/var/www/html")
}

func TestDockerExecutorComposerCommand(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	cmd := exec.ComposerCommand(t.Context(), "install", "--no-interaction")
	assert.Contains(t, cmd.Path, "docker")
	assert.Contains(t, cmd.Args, "compose")
	assert.Contains(t, cmd.Args, "exec")
	assert.Contains(t, cmd.Args, "web")
	assert.Contains(t, cmd.Args, "composer")
	assert.Contains(t, cmd.Args, "install")
	assert.Contains(t, cmd.Args, "--no-interaction")
}

func TestDockerExecutorPHPCommand(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	cmd := exec.PHPCommand(t.Context(), "-v")
	assert.Contains(t, cmd.Path, "docker")
	assert.Contains(t, cmd.Args, "compose")
	assert.Contains(t, cmd.Args, "exec")
	assert.Contains(t, cmd.Args, "web")
	assert.Contains(t, cmd.Args, "php")
	assert.Contains(t, cmd.Args, "-v")
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

	cmd := withEnv.PHPCommand(t.Context(), "vendor/bin/shopware-deployment-helper", "run")
	assert.Contains(t, cmd.Env, "INSTALL_LOCALE=de-DE")
	assert.Contains(t, cmd.Env, "INSTALL_CURRENCY=EUR")
}

func TestLocalExecutorWithoutEnv(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}

	cmd := exec.PHPCommand(t.Context(), "-v")
	assert.NotNil(t, cmd.Env)
	assert.Contains(t, cmd.Env, "PROJECT_ROOT=/project")
}

func TestDockerExecutorWithEnv(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}
	withEnv := exec.WithEnv(map[string]string{
		"INSTALL_LOCALE": "en-GB",
	})

	cmd := withEnv.PHPCommand(t.Context(), "vendor/bin/shopware-deployment-helper", "run")
	assert.Contains(t, cmd.Args, "-e")
	assert.Contains(t, cmd.Args, "INSTALL_LOCALE=en-GB")
}

func TestSymfonyCLIExecutorWithEnv(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}
	withEnv := exec.WithEnv(map[string]string{
		"INSTALL_LOCALE": "de-DE",
	})

	cmd := withEnv.PHPCommand(t.Context(), "-v")
	assert.Contains(t, cmd.Env, "INSTALL_LOCALE=de-DE")
}

func TestLocalExecutorNPMCommand(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}

	cmd := exec.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, []string{"npm", "run", "dev"}, cmd.Args)
	assert.Equal(t, "/project", cmd.Dir)
}

func TestDockerExecutorNPMCommand(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	cmd := exec.NPMCommand(t.Context(), "run", "dev")
	assert.Contains(t, cmd.Args, "compose")
	assert.Contains(t, cmd.Args, "exec")
	assert.Contains(t, cmd.Args, "web")
	assert.Contains(t, cmd.Args, "npm")
	assert.Contains(t, cmd.Args, "run")
	assert.Contains(t, cmd.Args, "dev")
}

func TestSymfonyCLIExecutorNPMCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}

	cmd := exec.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, []string{"npm", "run", "dev"}, cmd.Args)
}

func TestLocalExecutorWithRelDir(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	withDir := exec.WithRelDir("vendor/shopware/administration/Resources/app/administration")

	cmd := withDir.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, "/project/vendor/shopware/administration/Resources/app/administration", cmd.Dir)

	cmd = withDir.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, "/project/vendor/shopware/administration/Resources/app/administration", cmd.Dir)
}

func TestDockerExecutorWithRelDir(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}

	cmd := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, "/project", cmd.Dir)
	assert.Contains(t, cmd.Args, "--workdir")
	assert.Contains(t, cmd.Args, "/var/www/html")

	withDir := exec.WithRelDir("vendor/shopware/administration/Resources/app/administration")

	cmd = withDir.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, "/project", cmd.Dir)
	assert.Contains(t, cmd.Args, "--workdir")
	assert.Contains(t, cmd.Args, "/var/www/html/vendor/shopware/administration/Resources/app/administration")
}

func TestSymfonyCLIExecutorWithRelDir(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}
	withDir := exec.WithRelDir("vendor/shopware/administration/Resources/app/administration")

	cmd := withDir.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, "/project/vendor/shopware/administration/Resources/app/administration", cmd.Dir)

	cmd = withDir.NPMCommand(t.Context(), "run", "dev")
	assert.Equal(t, "/project/vendor/shopware/administration/Resources/app/administration", cmd.Dir)
}

func TestWithRelDirPreservesEnv(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	withEnv := exec.WithEnv(map[string]string{"FOO": "bar"})
	withDirAndEnv := withEnv.WithRelDir("subdir")

	cmd := withDirAndEnv.PHPCommand(t.Context(), "-v")
	assert.Equal(t, "/project/subdir", cmd.Dir)
	assert.Contains(t, cmd.Env, "FOO=bar")
}

func TestWithEnvPreservesRelDir(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	withDir := exec.WithRelDir("subdir")
	withDirAndEnv := withDir.WithEnv(map[string]string{"FOO": "bar"})

	cmd := withDirAndEnv.PHPCommand(t.Context(), "-v")
	assert.Equal(t, "/project/subdir", cmd.Dir)
	assert.Contains(t, cmd.Env, "FOO=bar")
}

func TestNewLocal(t *testing.T) {
	exec := NewLocal("/my/project")

	cmd := exec.NPMCommand(t.Context(), "install")
	assert.Equal(t, "/my/project", cmd.Dir)
	assert.Equal(t, []string{"npm", "install"}, cmd.Args)
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

package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestNewLocalExecutor(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")

	cfg := &shop.EnvironmentConfig{Type: "local"}

	exec, err := New(cfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, "local", exec.Type())
}

func TestNewLocalExecutorEmptyType(t *testing.T) {
	t.Setenv("SHOPWARE_CLI_NO_SYMFONY_CLI", "1")

	cfg := &shop.EnvironmentConfig{Type: ""}

	exec, err := New(cfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, "local", exec.Type())
}

func TestNewDockerExecutor(t *testing.T) {
	cfg := &shop.EnvironmentConfig{Type: "docker"}

	exec, err := New(cfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, "docker", exec.Type())
}

func TestNewUnsupportedType(t *testing.T) {
	cfg := &shop.EnvironmentConfig{Type: "unknown"}

	_, err := New(cfg, &shop.Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported environment type: unknown")
}

func TestLocalExecutorConsoleCommand(t *testing.T) {
	exec := &LocalExecutor{}

	cmd := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, []string{"php", "bin/console", "cache:clear"}, cmd.Args)
}

func TestLocalExecutorComposerCommand(t *testing.T) {
	exec := &LocalExecutor{}

	cmd := exec.ComposerCommand(t.Context(), "install")
	assert.Equal(t, []string{"composer", "install"}, cmd.Args)
}

func TestLocalExecutorPHPCommand(t *testing.T) {
	exec := &LocalExecutor{}

	cmd := exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, []string{"php", "-v"}, cmd.Args)
}

func TestSymfonyCLIExecutorConsoleCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony"}

	cmd := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, []string{"/usr/local/bin/symfony", "php", "bin/console", "cache:clear"}, cmd.Args)
}

func TestSymfonyCLIExecutorComposerCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony"}

	cmd := exec.ComposerCommand(t.Context(), "install")
	assert.Equal(t, []string{"/usr/local/bin/symfony", "composer", "install"}, cmd.Args)
}

func TestSymfonyCLIExecutorPHPCommand(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony"}

	cmd := exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, []string{"/usr/local/bin/symfony", "php", "-v"}, cmd.Args)
}

func TestDockerExecutorConsoleCommand(t *testing.T) {
	exec := &DockerExecutor{}

	cmd := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Contains(t, cmd.Path, "docker")
	assert.Contains(t, cmd.Args, "compose")
	assert.Contains(t, cmd.Args, "exec")
	assert.Contains(t, cmd.Args, "web")
	assert.Contains(t, cmd.Args, "php")
	assert.Contains(t, cmd.Args, "bin/console")
	assert.Contains(t, cmd.Args, "cache:clear")
}

func TestDockerExecutorComposerCommand(t *testing.T) {
	exec := &DockerExecutor{}

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
	exec := &DockerExecutor{}

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
	exec := &LocalExecutor{}
	ctx := WithEnv(t.Context(), map[string]string{
		"INSTALL_LOCALE":   "de-DE",
		"INSTALL_CURRENCY": "EUR",
	})

	cmd := exec.PHPCommand(ctx, "vendor/bin/shopware-deployment-helper", "run")
	assert.Contains(t, cmd.Env, "INSTALL_LOCALE=de-DE")
	assert.Contains(t, cmd.Env, "INSTALL_CURRENCY=EUR")
}

func TestLocalExecutorWithoutEnv(t *testing.T) {
	exec := &LocalExecutor{}

	cmd := exec.PHPCommand(t.Context(), "-v")
	assert.Nil(t, cmd.Env)
}

func TestDockerExecutorWithEnv(t *testing.T) {
	exec := &DockerExecutor{}
	ctx := WithEnv(t.Context(), map[string]string{
		"INSTALL_LOCALE": "en-GB",
	})

	cmd := exec.PHPCommand(ctx, "vendor/bin/shopware-deployment-helper", "run")
	assert.Contains(t, cmd.Args, "-e")
	assert.Contains(t, cmd.Args, "INSTALL_LOCALE=en-GB")
}

func TestSymfonyCLIExecutorWithEnv(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony"}
	ctx := WithEnv(t.Context(), map[string]string{
		"INSTALL_LOCALE": "de-DE",
	})

	cmd := exec.PHPCommand(ctx, "-v")
	assert.Contains(t, cmd.Env, "INSTALL_LOCALE=de-DE")
}

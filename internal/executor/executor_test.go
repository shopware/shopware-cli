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

	p := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Equal(t, []string{"php", "bin/console", "cache:clear"}, p.Cmd.Args)
	assert.Equal(t, "/project", p.Cmd.Dir)
}

func TestLocalExecutorComposerCommand(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}

	p := exec.ComposerCommand(t.Context(), "install")
	assert.Equal(t, []string{"composer", "install"}, p.Cmd.Args)
	assert.Equal(t, "/project", p.Cmd.Dir)
}

func TestLocalExecutorPHPCommand(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}

	p := exec.PHPCommand(t.Context(), "-v")
	assert.Equal(t, []string{"php", "-v"}, p.Cmd.Args)
	assert.Equal(t, "/project", p.Cmd.Dir)
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

func TestNewSSHExecutor(t *testing.T) {
	cfg := &shop.EnvironmentConfig{
		Type: "ssh",
		SSH: &shop.EnvironmentSSHConfig{
			Host:       "example.com",
			User:       "deploy",
			DeployPath: "/var/www/shop",
		},
	}

	exec, err := New("/project", cfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, TypeSSH, exec.Type())
}

func TestNewSSHExecutorRequiresSSHSection(t *testing.T) {
	cfg := &shop.EnvironmentConfig{Type: "ssh"}

	_, err := New("/project", cfg, &shop.Config{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssh: section")
}

func TestLocalExecutorDeployerNil(t *testing.T) {
	exec := &LocalExecutor{projectRoot: "/project"}
	assert.Nil(t, exec.Deployer())
}

func TestDockerExecutorDeployerNil(t *testing.T) {
	exec := &DockerExecutor{projectRoot: "/project"}
	assert.Nil(t, exec.Deployer())
}

func TestSymfonyCLIExecutorDeployerNil(t *testing.T) {
	exec := &SymfonyCLIExecutor{BinaryPath: "/usr/local/bin/symfony", projectRoot: "/project"}
	assert.Nil(t, exec.Deployer())
}

func TestSSHExecutorDeployer(t *testing.T) {
	envCfg := &shop.EnvironmentConfig{
		Type: "ssh",
		SSH: &shop.EnvironmentSSHConfig{
			Host:       "example.com",
			User:       "deploy",
			DeployPath: "/var/www/shop",
		},
	}
	exec := &SSHExecutor{projectRoot: "/project", envCfg: envCfg}

	d := exec.Deployer()
	assert.NotNil(t, d)
}

func TestSSHExecutorDeployerNilWithoutDeployPath(t *testing.T) {
	envCfg := &shop.EnvironmentConfig{
		Type: "ssh",
		SSH: &shop.EnvironmentSSHConfig{
			Host: "example.com",
		},
	}
	exec := &SSHExecutor{projectRoot: "/project", envCfg: envCfg}

	assert.Nil(t, exec.Deployer())
}

func TestSSHExecutorConsoleCommand(t *testing.T) {
	envCfg := &shop.EnvironmentConfig{
		Type: "ssh",
		SSH: &shop.EnvironmentSSHConfig{
			Host:         "example.com",
			User:         "deploy",
			Port:         2222,
			IdentityFile: "/keys/id",
			DeployPath:   "/var/www/shop",
		},
	}
	exec := &SSHExecutor{projectRoot: "/project", envCfg: envCfg}

	p := exec.ConsoleCommand(t.Context(), "cache:clear")
	assert.Contains(t, p.Cmd.Path, "ssh")
	assert.Contains(t, p.Cmd.Args, "-p")
	assert.Contains(t, p.Cmd.Args, "2222")
	assert.Contains(t, p.Cmd.Args, "-i")
	assert.Contains(t, p.Cmd.Args, "/keys/id")
	assert.Contains(t, p.Cmd.Args, "deploy@example.com")
	remoteCmd := p.Cmd.Args[len(p.Cmd.Args)-1]
	assert.Contains(t, remoteCmd, "cd '/var/www/shop/current'")
	assert.Contains(t, remoteCmd, "'php' 'bin/console' 'cache:clear'")
}

func TestSSHExecutorWithRelDirOnRemote(t *testing.T) {
	envCfg := &shop.EnvironmentConfig{
		Type: "ssh",
		SSH:  &shop.EnvironmentSSHConfig{Host: "example.com", DeployPath: "/srv/shop"},
	}
	exec := &SSHExecutor{projectRoot: "/project", envCfg: envCfg}
	withDir := exec.WithRelDir("custom/plugins/MyPlugin")

	p := withDir.PHPCommand(t.Context(), "-v")
	remoteCmd := p.Cmd.Args[len(p.Cmd.Args)-1]
	assert.Contains(t, remoteCmd, "cd '/srv/shop/current/custom/plugins/MyPlugin'")
}

func TestSSHExecutorWithEnvOnRemote(t *testing.T) {
	envCfg := &shop.EnvironmentConfig{
		Type: "ssh",
		SSH:  &shop.EnvironmentSSHConfig{Host: "example.com", DeployPath: "/srv/shop"},
	}
	exec := &SSHExecutor{projectRoot: "/project", envCfg: envCfg}
	withEnv := exec.WithEnv(map[string]string{"APP_ENV": "prod"})

	p := withEnv.PHPCommand(t.Context(), "-v")
	remoteCmd := p.Cmd.Args[len(p.Cmd.Args)-1]
	assert.Contains(t, remoteCmd, "APP_ENV='prod'")
}

func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	assert.Equal(t, `'foo'`, shellQuote("foo"))
	assert.Equal(t, `'O'\''Brien'`, shellQuote("O'Brien"))
}

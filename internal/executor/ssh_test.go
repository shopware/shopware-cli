package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func newTestSSHExecutor(t *testing.T) Executor {
	t.Helper()

	old := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = old })

	envCfg := &shop.EnvironmentConfig{
		Type: TypeSSH,
		SSH: &shop.EnvironmentSSH{
			Host:         "shop.example.com",
			Port:         2222,
			User:         "deploy",
			IdentityFile: "/keys/id_ed25519",
		},
		Deployment: &shop.EnvironmentDeployment{Path: "/var/www/shopware/"},
	}

	exec, err := New("/local/project", envCfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, TypeSSH, exec.Type())

	return exec
}

func TestSSHExecutorConsoleCommand(t *testing.T) {
	exec := newTestSSHExecutor(t)

	p := exec.ConsoleCommand(t.Context(), "cache:clear")

	assert.Equal(t, []string{
		"ssh",
		"-p", "2222",
		"-i", "/keys/id_ed25519",
		"deploy@shop.example.com",
		"cd '/var/www/shopware/current' && 'php' 'bin/console' 'cache:clear'",
	}, p.Cmd.Args)
}

func TestSSHExecutorCommandsUseRemoteBinaries(t *testing.T) {
	exec := newTestSSHExecutor(t)

	lastArg := func(p *Process) string {
		return p.Cmd.Args[len(p.Cmd.Args)-1]
	}

	cases := map[string]string{
		"composer": lastArg(exec.ComposerCommand(t.Context(), "install")),
		"php":      lastArg(exec.PHPCommand(t.Context(), "-v")),
		"npm":      lastArg(exec.NPMCommand(t.Context(), "ci")),
	}

	assert.Contains(t, cases["composer"], "'composer' 'install'")
	assert.Contains(t, cases["php"], "'php' '-v'")
	assert.Contains(t, cases["npm"], "'npm' 'ci'")

	for _, remote := range cases {
		assert.Contains(t, remote, "cd '/var/www/shopware/current' && ")
	}
}

func TestSSHExecutorWithEnvAndRelDir(t *testing.T) {
	exec := newTestSSHExecutor(t)

	exec = exec.WithEnv(map[string]string{
		"APP_ENV":      "prod",
		"PROJECT_ROOT": "/local/project",
	}).WithRelDir("custom/plugins")

	p := exec.ConsoleCommand(t.Context(), "cache:clear")
	remote := p.Cmd.Args[len(p.Cmd.Args)-1]

	// env variables are sorted, PROJECT_ROOT is mapped to the remote release
	assert.Equal(t, "cd '/var/www/shopware/current/custom/plugins' && APP_ENV='prod' PROJECT_ROOT='/var/www/shopware/current' 'php' 'bin/console' 'cache:clear'", remote)
}

func TestSSHExecutorNormalizePath(t *testing.T) {
	exec := newTestSSHExecutor(t)

	assert.Equal(t, "/var/www/shopware/current/custom/plugins/Foo", exec.NormalizePath("/local/project/custom/plugins/Foo"))
}

func TestSSHExecutorQuotesArguments(t *testing.T) {
	exec := newTestSSHExecutor(t)

	p := exec.ConsoleCommand(t.Context(), "system:config:set", "core.basicInformation.shopName", "it's a shop")
	remote := p.Cmd.Args[len(p.Cmd.Args)-1]

	assert.Contains(t, remote, `'system:config:set' 'core.basicInformation.shopName' 'it'\''s a shop'`)
}

func TestSSHExecutorHostKeyOptions(t *testing.T) {
	old := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = old })

	envCfg := &shop.EnvironmentConfig{
		Type: TypeSSH,
		SSH: &shop.EnvironmentSSH{
			Host:                  "shop.example.com",
			InsecureIgnoreHostKey: true,
		},
		Deployment: &shop.EnvironmentDeployment{Path: "/var/www/shopware"},
	}

	exec, err := New("", envCfg, &shop.Config{})
	assert.NoError(t, err)

	p := exec.PHPCommand(t.Context(), "-v")

	assert.Equal(t, []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"shop.example.com",
		"cd '/var/www/shopware/current' && 'php' '-v'",
	}, p.Cmd.Args)
}

func TestSSHExecutorRequiresConfig(t *testing.T) {
	_, err := New("", &shop.EnvironmentConfig{Type: TypeSSH}, &shop.Config{})
	assert.ErrorContains(t, err, "ssh.host")

	_, err = New("", &shop.EnvironmentConfig{Type: TypeSSH, SSH: &shop.EnvironmentSSH{Host: "example.com"}}, &shop.Config{})
	assert.ErrorContains(t, err, "deployment.path")
}

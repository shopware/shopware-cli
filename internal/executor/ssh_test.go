package executor

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
	"github.com/shopware/shopware-cli/internal/sshcmd"
)

func stubSSHEnvironment(t *testing.T) {
	t.Helper()

	oldTerminal := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }

	oldSocketDir := sshcmd.ControlSocketDir
	sshcmd.ControlSocketDir = func() string { return "/home/user/.ssh" }

	t.Cleanup(func() {
		stdinIsTerminal = oldTerminal
		sshcmd.ControlSocketDir = oldSocketDir
	})
}

// controlMasterArgs are the multiplexing arguments expected by default; on
// Windows ControlMaster is unsupported and never emitted.
var controlMasterArgs = func() []string {
	if runtime.GOOS == "windows" {
		return nil
	}

	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=/home/user/.ssh/shopware-cli-%C",
		"-o", "ControlPersist=60",
	}
}()

func newTestSSHExecutor(t *testing.T) Executor {
	t.Helper()

	stubSSHEnvironment(t)

	envCfg := &shop.EnvironmentConfig{
		Type: TypeSSH,
		SSH: &shop.EnvironmentSSH{
			Host:         "shop.example.com",
			Port:         2222,
			User:         "deploy",
			IdentityFile: "/keys/id_ed25519",
			Deployment:   &shop.EnvironmentDeployment{Path: "/var/www/shopware/"},
		},
	}

	exec, err := New("/local/project", envCfg, &shop.Config{})
	assert.NoError(t, err)
	assert.Equal(t, TypeSSH, exec.Type())

	return exec
}

func TestSSHExecutorConsoleCommand(t *testing.T) {
	exec := newTestSSHExecutor(t)

	p := exec.ConsoleCommand(t.Context(), "cache:clear")

	expected := []string{
		"ssh",
		"-p", "2222",
		"-i", "/keys/id_ed25519",
	}
	expected = append(expected, controlMasterArgs...)
	expected = append(expected,
		"deploy@shop.example.com",
		"cd '/var/www/shopware/current' && 'php' 'bin/console' 'cache:clear'",
	)

	assert.Equal(t, expected, p.Cmd.Args)
}

func TestSSHExecutorControlMasterCanBeDisabled(t *testing.T) {
	stubSSHEnvironment(t)

	disabled := false
	envCfg := &shop.EnvironmentConfig{
		Type: TypeSSH,
		SSH: &shop.EnvironmentSSH{
			Host:          "shop.example.com",
			ControlMaster: &disabled,
			Deployment:    &shop.EnvironmentDeployment{Path: "/var/www/shopware"},
		},
	}

	exec, err := New("", envCfg, &shop.Config{})
	assert.NoError(t, err)

	p := exec.PHPCommand(t.Context(), "-v")

	assert.Equal(t, []string{
		"ssh",
		"shop.example.com",
		"cd '/var/www/shopware/current' && 'php' '-v'",
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
	stubSSHEnvironment(t)

	envCfg := &shop.EnvironmentConfig{
		Type: TypeSSH,
		SSH: &shop.EnvironmentSSH{
			Host:                  "shop.example.com",
			InsecureIgnoreHostKey: true,
			Deployment:            &shop.EnvironmentDeployment{Path: "/var/www/shopware"},
		},
	}

	exec, err := New("", envCfg, &shop.Config{})
	assert.NoError(t, err)

	p := exec.PHPCommand(t.Context(), "-v")

	expected := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}
	expected = append(expected, controlMasterArgs...)
	expected = append(expected,
		"shop.example.com",
		"cd '/var/www/shopware/current' && 'php' '-v'",
	)

	assert.Equal(t, expected, p.Cmd.Args)
}

func TestSSHExecutorRequiresConfig(t *testing.T) {
	_, err := New("", &shop.EnvironmentConfig{Type: TypeSSH}, &shop.Config{})
	assert.ErrorContains(t, err, "ssh.host")

	_, err = New("", &shop.EnvironmentConfig{Type: TypeSSH, SSH: &shop.EnvironmentSSH{Host: "example.com"}}, &shop.Config{})
	assert.ErrorContains(t, err, "deployment.path")
}

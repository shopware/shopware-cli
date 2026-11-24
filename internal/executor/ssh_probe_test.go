package executor

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/shop"
)

func TestParseSSHProbe(t *testing.T) {
	assert.Equal(t, SSHProbe{}, parseSSHProbe("Welcome to the server\n"))
	assert.Equal(t,
		SSHProbe{Connected: true, ParentDirectory: true},
		parseSSHProbe("motd banner\nconnected\nparent-directory\n"))
	assert.Equal(t,
		SSHProbe{Connected: true, ParentDirectory: true, Directory: true, PHP: true, Console: true, Logs: true, DeploymentHelper: true},
		parseSSHProbe("connected\r\nparent-directory\ndirectory\nphp\nconsole\nlogs\ndeployment-helper\n"))
}

func TestSSHProbeScriptQuotesConfiguredValues(t *testing.T) {
	e := &SSHExecutor{host: "shop.example.com", directory: "/srv/my shop/current", phpBinary: "/opt/php 8.3/bin/php", phpArgs: []string{"-d", "memory_limit=512M"}}

	script := e.probeScript("deploy")

	assert.Contains(t, script, "test -d '/srv/my shop' && echo parent-directory")
	assert.Contains(t, script, "cd '/srv/my shop/current' 2>/dev/null || exit 0")
	assert.Contains(t, script, "'/opt/php 8.3/bin/php' -d memory_limit=512M -r 'exit(0);'")
	assert.Contains(t, script, "sudo -n -H -u deploy -- '/opt/php 8.3/bin/php' -d memory_limit=512M -r 'exit(0);'")
}

func fakeSSH(t *testing.T, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses POSIX shell")
	}
	bin := t.TempDir()
	args := filepath.Join(bin, "args")
	require.NoError(t, os.WriteFile(filepath.Join(bin, "ssh"), []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > "+args+"\n"+script+"\n"), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return args
}

func TestProbeSSHReportsWhatTheRemoteHostHas(t *testing.T) {
	args := fakeSSH(t, "printf 'connected\\nparent-directory\\ndirectory\\nphp\\nconsole\\n'")

	probe := ProbeSSH(t.Context(), &shop.EnvironmentSSHConfig{Host: "shop.example.com", User: "deploy", Port: 2222, Directory: "/srv/shop/current"})

	assert.True(t, probe.Connected)
	assert.True(t, probe.Console)
	assert.False(t, probe.DeploymentPHP)
	assert.False(t, probe.DeploymentHelper)
	assert.Empty(t, probe.Error)

	sent, err := os.ReadFile(args)
	require.NoError(t, err)
	// The probe must never wait for a password or passphrase.
	assert.Contains(t, string(sent), "BatchMode=yes\n")
	assert.Contains(t, string(sent), "-p\n2222\n")
	assert.Contains(t, string(sent), "deploy@shop.example.com\n")
}

func TestProbeSSHChecksDeploymentPHPBeforeTheFirstRollout(t *testing.T) {
	fakeSSH(t, "printf 'connected\\ndeployment-php\\nparent-directory\\n'")

	probe := ProbeSSH(t.Context(), &shop.EnvironmentSSHConfig{Host: "shop.example.com", Directory: "/srv/shop/current", Become: "deploy"})
	assert.True(t, probe.Connected)
	assert.True(t, probe.DeploymentPHP)
	assert.False(t, probe.Directory)
}

func TestProbeSSHKeepsTheClientErrorWhenUnreachable(t *testing.T) {
	fakeSSH(t, "echo 'ssh: connect to host shop.example.com port 22: Connection refused' >&2\nexit 255")

	probe := ProbeSSH(t.Context(), &shop.EnvironmentSSHConfig{Host: "shop.example.com", Directory: "/srv/shop/current"})

	assert.False(t, probe.Connected)
	assert.Equal(t, "ssh: connect to host shop.example.com port 22: Connection refused", probe.Error)
}

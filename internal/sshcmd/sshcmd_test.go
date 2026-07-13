package sshcmd

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/shopware/shopware-cli/internal/shop"
)

func stubEnvironment(t *testing.T) {
	t.Helper()

	oldSocketDir := ControlSocketDir
	ControlSocketDir = func() string { return "/home/user/.ssh" }

	oldSshpass := sshpassPath
	sshpassPath = func() string { return "" }

	t.Cleanup(func() {
		ControlSocketDir = oldSocketDir
		sshpassPath = oldSshpass
	})
}

func expectedControlMasterArgs() []string {
	if runtime.GOOS == "windows" {
		return nil
	}

	return []string{
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=/home/user/.ssh/shopware-cli-%C",
		"-o", "ControlPersist=60",
	}
}

func TestBaseArgs(t *testing.T) {
	stubEnvironment(t)

	cfg := &shop.EnvironmentSSH{
		Host:         "shop.example.com",
		Port:         2222,
		IdentityFile: "/keys/id_ed25519",
	}

	expected := []string{"-p", "2222", "-i", "/keys/id_ed25519"}
	expected = append(expected, expectedControlMasterArgs()...)

	assert.Equal(t, expected, BaseArgs(cfg))
}

func TestBaseArgsControlMasterDisabled(t *testing.T) {
	stubEnvironment(t)

	disabled := false
	cfg := &shop.EnvironmentSSH{Host: "shop.example.com", ControlMaster: &disabled}

	assert.Empty(t, BaseArgs(cfg))
}

func TestBaseArgsHostKeyOptions(t *testing.T) {
	stubEnvironment(t)

	disabled := false
	cfg := &shop.EnvironmentSSH{
		Host:                  "shop.example.com",
		KnownHostsFile:        "/known_hosts",
		InsecureIgnoreHostKey: true,
		ControlMaster:         &disabled,
	}

	assert.Equal(t, []string{
		"-o", "UserKnownHostsFile=/known_hosts",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
	}, BaseArgs(cfg))
}

func TestDestination(t *testing.T) {
	assert.Equal(t, "deploy@shop.example.com", Destination(&shop.EnvironmentSSH{Host: "shop.example.com", User: "deploy"}))
	assert.Equal(t, "shop.example.com", Destination(&shop.EnvironmentSSH{Host: "shop.example.com"}))
}

func TestBuild(t *testing.T) {
	stubEnvironment(t)

	disabled := false
	cfg := &shop.EnvironmentSSH{Host: "shop.example.com", User: "deploy", ControlMaster: &disabled}

	cmd := Build(t.Context(), cfg, "uptime", "-t")

	assert.Equal(t, []string{"ssh", "-t", "deploy@shop.example.com", "uptime"}, cmd.Args)
}

func TestBuildUsesSshpassForPasswords(t *testing.T) {
	stubEnvironment(t)
	sshpassPath = func() string { return "/usr/bin/sshpass" }

	disabled := false
	cfg := &shop.EnvironmentSSH{Host: "shop.example.com", Password: "secret", ControlMaster: &disabled}

	cmd := Build(t.Context(), cfg, "uptime")

	assert.Equal(t, []string{"/usr/bin/sshpass", "-e", "ssh", "shop.example.com", "uptime"}, cmd.Args)
	assert.Contains(t, cmd.Env, "SSHPASS=secret")
}

func TestBuildFallsBackToInteractivePassword(t *testing.T) {
	stubEnvironment(t)

	disabled := false
	cfg := &shop.EnvironmentSSH{Host: "shop.example.com", Password: "secret", ControlMaster: &disabled}

	cmd := Build(t.Context(), cfg, "uptime")

	assert.Equal(t, []string{"ssh", "shop.example.com", "uptime"}, cmd.Args)
}

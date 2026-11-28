package environment

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

func sshEnv() *shop.EnvironmentConfig {
	return &shop.EnvironmentConfig{Type: "ssh", URL: "https://shop.example.com", SSH: &shop.EnvironmentSSHConfig{
		Host: "shop.example.com", User: "deploy", Directory: "/srv/shop/current",
	}}
}

func probing(probe executor.SSHProbe) SSHProber {
	return func(context.Context, *shop.EnvironmentSSHConfig) executor.SSHProbe { return probe }
}

func checkStates(result Result) map[string]CheckState {
	states := make(map[string]CheckState, len(result.Checks))
	for _, check := range result.Checks {
		states[check.Name] = check.State
	}
	return states
}

func TestSSHEnvironment(t *testing.T) {
	project := executor.SSHProbe{Connected: true, ParentDirectory: true, Directory: true, PHP: true, Console: true, Logs: true, DeploymentHelper: true}
	without := func(change func(*executor.SSHProbe)) executor.SSHProbe {
		probe := project
		change(&probe)
		return probe
	}

	for name, tc := range map[string]struct {
		probe          executor.SSHProbe
		ready, limited bool
		fix            Setting
		states         map[string]CheckState
	}{
		"complete project": {
			probe: project, ready: true,
			states: map[string]CheckState{CheckConnection: CheckOK, CheckConfiguration: CheckOK, CheckCapabilities: CheckOK},
		},
		"connection refused": {
			probe: executor.SSHProbe{Error: "ssh: connect to host shop.example.com port 22: Connection refused"}, fix: SettingPort,
			states: map[string]CheckState{CheckConnection: CheckFail, CheckConfiguration: CheckSkipped, CheckCapabilities: CheckSkipped},
		},
		"unknown host": {
			probe: executor.SSHProbe{Error: "ssh: Could not resolve hostname shop.example.com: nodename nor servname provided"}, fix: SettingHost,
			states: map[string]CheckState{CheckConnection: CheckFail, CheckConfiguration: CheckSkipped, CheckCapabilities: CheckSkipped},
		},
		"rejected key": {
			probe: executor.SSHProbe{Error: "deploy@shop.example.com: Permission denied (publickey)."}, fix: SettingUser,
			states: map[string]CheckState{CheckConnection: CheckFail, CheckConfiguration: CheckSkipped, CheckCapabilities: CheckSkipped},
		},
		"directory and parent missing": {
			probe: executor.SSHProbe{Connected: true}, fix: SettingDirectory,
			states: map[string]CheckState{CheckConnection: CheckOK, CheckConfiguration: CheckFail, CheckCapabilities: CheckSkipped},
		},
		"not deployed yet": {
			probe: executor.SSHProbe{Connected: true, ParentDirectory: true}, ready: true, limited: true,
			states: map[string]CheckState{CheckConnection: CheckOK, CheckConfiguration: CheckOK, CheckCapabilities: CheckWarn},
		},
		"php missing": {
			probe: without(func(p *executor.SSHProbe) { p.PHP = false }), fix: SettingPHPBinary,
			states: map[string]CheckState{CheckConnection: CheckOK, CheckConfiguration: CheckOK, CheckCapabilities: CheckFail},
		},
		"not a shopware project": {
			probe: without(func(p *executor.SSHProbe) { p.Console = false }), fix: SettingDirectory,
			states: map[string]CheckState{CheckConnection: CheckOK, CheckConfiguration: CheckFail, CheckCapabilities: CheckSkipped},
		},
		"deployment helper missing": {
			probe: without(func(p *executor.SSHProbe) { p.DeploymentHelper = false }), ready: true, limited: true,
			states: map[string]CheckState{CheckConnection: CheckOK, CheckConfiguration: CheckOK, CheckCapabilities: CheckWarn},
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := Test(t.Context(), "production", sshEnv(), probing(tc.probe))

			assert.Equal(t, tc.ready, result.Ready)
			assert.Equal(t, tc.limited, result.Limited)
			assert.Equal(t, tc.fix, result.Fix)
			assert.Equal(t, tc.states, checkStates(result))
			// A failure always says why, so the list can show it next to NOT READY.
			assert.Equal(t, !tc.ready, result.Reason != "")
			assert.Equal(t, "ssh · deploy@shop.example.com:22", result.Target)
		})
	}
}

func TestSSHEnvironmentListsAvailableCapabilities(t *testing.T) {
	result := Test(t.Context(), "production", sshEnv(), probing(executor.SSHProbe{
		Connected: true, ParentDirectory: true, Directory: true, PHP: true, Console: true, Logs: true, DeploymentHelper: true,
	}))
	require.Len(t, result.Checks, 3)
	assert.Equal(t, "dump, sql, console, logs", result.Checks[2].Summary)

	result = Test(t.Context(), "production", sshEnv(), probing(executor.SSHProbe{
		Connected: true, ParentDirectory: true, Directory: true, PHP: true, Console: true, Logs: true,
	}))
	assert.Equal(t, "console, logs only", result.Checks[2].Summary)
}

func TestSSHEnvironmentWithoutHostIsNotProbed(t *testing.T) {
	env := sshEnv()
	env.SSH.Host = ""

	result := Test(t.Context(), "production", env, func(context.Context, *shop.EnvironmentSSHConfig) executor.SSHProbe {
		t.Fatal("an incomplete configuration must not open a connection")
		return executor.SSHProbe{}
	})

	assert.False(t, result.Ready)
	assert.Equal(t, CheckFail, checkStates(result)[CheckConfiguration])
}

func TestSSHHostGroupNeedsEveryHost(t *testing.T) {
	env := &shop.EnvironmentConfig{Type: "ssh", SSH: &shop.EnvironmentSSHConfig{
		Directory:     "/srv/shop/current",
		MigrationHost: "web1",
		Hosts: map[string]*shop.EnvironmentSSHHostConfig{
			"web1": {Host: "web1.example.com"},
			"web2": {Host: "web2.example.com"},
		},
	}}
	project := executor.SSHProbe{Connected: true, ParentDirectory: true, Directory: true, PHP: true, Console: true, Logs: true, DeploymentHelper: true}

	var probed []string
	result := Test(t.Context(), "production", env, func(_ context.Context, cfg *shop.EnvironmentSSHConfig) executor.SSHProbe {
		probed = append(probed, cfg.Host)
		return project
	})
	assert.ElementsMatch(t, []string{"web1.example.com", "web2.example.com"}, probed)
	assert.True(t, result.Ready)
	// Commands other than deployments cannot target a host group.
	assert.True(t, result.Limited)
	assert.Equal(t, "ssh · 2 hosts", result.Target)

	result = Test(t.Context(), "production", env, func(_ context.Context, cfg *shop.EnvironmentSSHConfig) executor.SSHProbe {
		if cfg.Host == "web2.example.com" {
			return executor.SSHProbe{Error: "ssh: connect to host web2.example.com port 22: Operation timed out"}
		}
		return project
	})
	assert.False(t, result.Ready)
	assert.Contains(t, result.Checks[0].Detail, "web2")
}

func TestSSHHostGroupChecksDeployedHostsWhileOthersAwaitRollout(t *testing.T) {
	env := &shop.EnvironmentConfig{Type: "ssh", SSH: &shop.EnvironmentSSHConfig{
		Directory:     "/srv/shop/current",
		MigrationHost: "web1",
		Hosts: map[string]*shop.EnvironmentSSHHostConfig{
			"web1": {Host: "web1.example.com"},
			"web2": {Host: "web2.example.com"},
		},
	}}
	for _, tc := range []struct{ php, console bool }{{false, true}, {true, false}, {true, true}} {
		result := Test(t.Context(), "production", env, func(_ context.Context, cfg *shop.EnvironmentSSHConfig) executor.SSHProbe {
			if cfg.Host == "web1.example.com" {
				return executor.SSHProbe{Connected: true, ParentDirectory: true}
			}
			return executor.SSHProbe{Connected: true, ParentDirectory: true, Directory: true, PHP: tc.php, Console: tc.console}
		})
		assert.Equal(t, tc.php && tc.console, result.Ready)
		assert.Equal(t, result.Ready, result.Limited)
	}
}

func TestSSHEnvironmentRequiresBothLoginAndDeploymentPHP(t *testing.T) {
	env := sshEnv()
	env.SSH.Become = "release"

	for _, tc := range []struct{ directory, php, deploymentPHP, ready bool }{
		{true, true, false, false},
		{true, false, true, false},
		{true, true, true, true},
		{false, false, false, false},
		{false, false, true, true},
	} {
		result := Test(t.Context(), "production", env, probing(executor.SSHProbe{
			Connected: true, ParentDirectory: true, Directory: tc.directory, PHP: tc.php, Console: tc.directory,
			DeploymentPHP: tc.deploymentPHP,
		}))
		assert.Equal(t, tc.ready, result.Ready, "%+v", tc)
	}
}

func TestLocalEnvironmentNeedsURL(t *testing.T) {
	noProbe := func(context.Context, *shop.EnvironmentSSHConfig) executor.SSHProbe {
		t.Fatal("local environments have nothing to connect to")
		return executor.SSHProbe{}
	}

	result := Test(t.Context(), "local", &shop.EnvironmentConfig{Type: "docker", URL: "http://localhost:8000"}, noProbe)
	assert.True(t, result.Ready)
	assert.Equal(t, "docker", result.Target)

	result = Test(t.Context(), "local", &shop.EnvironmentConfig{}, noProbe)
	assert.False(t, result.Ready)
	assert.Equal(t, SettingURL, result.Fix)
	assert.Equal(t, "local", result.Target)
}

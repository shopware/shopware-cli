// Package environment tests the environments of a project config and remembers
// the outcome per machine.
package environment

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopware/shopware-cli/internal/executor"
	"github.com/shopware/shopware-cli/internal/shop"
)

type CheckState string

const (
	CheckOK      CheckState = "ok"
	CheckWarn    CheckState = "warn"
	CheckFail    CheckState = "fail"
	CheckSkipped CheckState = "skipped"
)

const (
	CheckConnection    = "Connection"
	CheckConfiguration = "Configuration"
	CheckCapabilities  = "Capabilities"
)

// Check is one row of a test result.
type Check struct {
	Name    string     `json:"name"`
	State   CheckState `json:"state"`
	Summary string     `json:"summary"`
	// Detail explains a warning or failure.
	Detail string `json:"detail,omitempty"`
}

// Result is the outcome of testing one environment.
type Result struct {
	Name   string  `json:"name"`
	Target string  `json:"target"`
	Checks []Check `json:"checks"`
	// Ready means commands and deployments can use the environment.
	Ready bool `json:"ready"`
	// Limited marks a ready environment that lacks some capabilities.
	Limited bool `json:"limited"`
	// Reason is the short cause shown next to NOT READY.
	Reason string `json:"reason,omitempty"`
	// Hint tells the user how to resolve a failure or limitation.
	Hint string `json:"hint,omitempty"`
	// Fix names the setting most likely to resolve a failure.
	Fix      Setting   `json:"-"`
	TestedAt time.Time `json:"tested_at"`
}

// SSHProber inspects the target of a single-host SSH configuration.
type SSHProber func(ctx context.Context, cfg *shop.EnvironmentSSHConfig) executor.SSHProbe

// Test checks connection, configuration and capabilities of one environment.
func Test(ctx context.Context, name string, env *shop.EnvironmentConfig, probe SSHProber) Result {
	result := Result{Name: name, Target: Target(env), TestedAt: time.Now()}

	if env.Type == executor.TypeSSH {
		testSSH(ctx, env, probe, &result)
	} else {
		testLocal(env, &result)
	}

	return result
}

// Target describes where an environment points, e.g. "ssh · deploy@shop.example.com:22".
func Target(env *shop.EnvironmentConfig) string {
	envType := env.Type
	if envType == "" {
		envType = executor.TypeLocal
	}
	if env.Type != executor.TypeSSH || env.SSH == nil {
		return envType
	}
	if len(env.SSH.Hosts) > 0 {
		return fmt.Sprintf("%s · %d hosts", envType, len(env.SSH.Hosts))
	}
	return envType + " · " + sshEndpoint(env.SSH)
}

func sshEndpoint(cfg *shop.EnvironmentSSHConfig) string {
	endpoint := cfg.Host
	if cfg.User != "" {
		endpoint = cfg.User + "@" + endpoint
	}
	port := cfg.Port
	if port == 0 {
		port = 22
	}
	return endpoint + ":" + strconv.Itoa(port)
}

func testLocal(env *shop.EnvironmentConfig, result *Result) {
	configuration := Check{Name: CheckConfiguration, State: CheckOK, Summary: "url set"}
	if env.URL == "" {
		configuration = Check{Name: CheckConfiguration, State: CheckFail, Summary: "url missing"}
		result.Reason, result.Fix = "url missing", SettingURL
		result.Hint = "Set the shop URL of this environment."
	}

	result.Checks = []Check{
		{Name: CheckConnection, State: CheckSkipped, Summary: "runs on this machine"},
		configuration,
		{Name: CheckCapabilities, State: CheckSkipped, Summary: "not checked for " + result.Target + " environments"},
	}
	result.Ready = configuration.State == CheckOK
}

func testSSH(ctx context.Context, env *shop.EnvironmentConfig, probe SSHProber, result *Result) {
	notRun := func(name string) Check { return Check{Name: name, State: CheckSkipped, Summary: "not run"} }

	hosts, _, err := env.SSH.ResolveHosts()
	if err != nil {
		result.Checks = []Check{
			notRun(CheckConnection),
			{Name: CheckConfiguration, State: CheckFail, Summary: "incomplete", Detail: err.Error()},
			notRun(CheckCapabilities),
		}
		result.Reason = "configuration incomplete"
		result.Hint = "Edit the environment and set the missing values."
		return
	}

	probes := make([]executor.SSHProbe, 0, len(hosts))
	var slowest time.Duration
	for _, host := range hosts {
		found := probe(ctx, host.Config)
		if !found.Connected {
			reason, hint, fix := classifySSHError(found.Error, host.Config)
			detail := reason
			if host.Name != "" {
				detail = host.Name + ": " + reason
			}
			result.Checks = []Check{
				{Name: CheckConnection, State: CheckFail, Summary: "unreachable", Detail: detail},
				notRun(CheckConfiguration),
				notRun(CheckCapabilities),
			}
			result.Reason, result.Hint, result.Fix = reason, hint, fix
			return
		}
		slowest = max(slowest, found.Duration)
		probes = append(probes, found)
	}

	reachable := fmt.Sprintf("reachable · %.1fs", slowest.Seconds())
	if len(hosts) > 1 {
		reachable = fmt.Sprintf("%d hosts reachable · %.1fs", len(hosts), slowest.Seconds())
	}
	connection := Check{Name: CheckConnection, State: CheckOK, Summary: reachable}

	set := "host and directory set"
	if env.URL != "" {
		set = "url, host and directory set"
	}

	configuration := Check{Name: CheckConfiguration, State: CheckOK, Summary: set}
	var undeployed string
	for i, found := range probes {
		host := hosts[i]
		check, hint, fix := sshHostFailure(host.Config, found)
		if check.State == CheckFail {
			if host.Name != "" {
				check.Detail = host.Name + ": " + check.Detail
			}
			result.Checks = []Check{connection, configuration, check}
			if check.Name == CheckConfiguration {
				result.Checks[1], result.Checks[2] = check, notRun(CheckCapabilities)
			}
			result.Reason, result.Hint, result.Fix = check.Summary, hint, fix
			return
		}
		if !found.Directory && undeployed == "" {
			undeployed = host.Config.Directory
			if host.Name != "" {
				undeployed = host.Name + ": " + undeployed
			}
		}
	}

	result.Ready = true
	if undeployed != "" {
		result.Checks = []Check{connection, configuration, {
			Name: CheckCapabilities, State: CheckWarn, Summary: "deployment pending",
			Detail: undeployed + " does not exist yet",
		}}
		result.Limited = true
		result.Hint = "Deploy the project to initialize the missing directory."
		return
	}

	if len(hosts) > 1 {
		result.Limited = true
		result.Checks = []Check{connection, configuration, {
			Name: CheckCapabilities, State: CheckWarn, Summary: "deployments only",
			Detail: "console, logs, dump and sql need a single-host environment",
		}}
		return
	}

	project := probes[0]
	capabilities := []string{"console"}
	var missing []string
	if project.DeploymentHelper {
		capabilities = append([]string{"dump", "sql"}, capabilities...)
	} else {
		missing = append(missing, "dump, sql: deployment-helper not found")
	}
	if project.Logs {
		capabilities = append(capabilities, "logs")
	} else {
		missing = append(missing, "logs: var/log not found")
	}

	capability := Check{Name: CheckCapabilities, State: CheckOK, Summary: strings.Join(capabilities, ", ")}
	if len(missing) > 0 {
		result.Limited = true
		capability.State = CheckWarn
		capability.Summary += " only"
		capability.Detail = strings.Join(missing, "; ")
		if !project.DeploymentHelper {
			result.Hint = "Run \"composer require shopware/deployment-helper\" in the project and deploy it to enable dump and sql. Everything else, including deploy, works."
		}
	}
	result.Checks = []Check{connection, configuration, capability}
}

// sshHostFailure checks required capabilities before group readiness is decided.
func sshHostFailure(cfg *shop.EnvironmentSSHConfig, found executor.SSHProbe) (Check, string, Setting) {
	switch {
	case !found.Directory && !found.ParentDirectory:
		return Check{Name: CheckConfiguration, State: CheckFail, Summary: "directory missing",
				Detail: cfg.Directory + " and its parent do not exist on the remote host"},
			"Edit Directory, or create the parent directory on the remote host. The first deployment creates the directory itself.", SettingDirectory
	case cfg.Become != "" && !found.DeploymentPHP:
		return Check{Name: CheckCapabilities, State: CheckFail, Summary: "deployment PHP unavailable",
				Detail: fmt.Sprintf("PHP could not run as %q through passwordless sudo", cfg.Become)},
			"Check ssh.become in the project config, the PHP binary, and the SSH login's sudo permissions.", ""
	case found.Directory && !found.PHP:
		php := cfg.PHPBinary
		if php == "" {
			php = "php"
		}
		return Check{Name: CheckCapabilities, State: CheckFail, Summary: "php not found",
				Detail: fmt.Sprintf("%q does not run as the SSH login", php)},
			"Edit PHP binary and point it at the PHP of the remote host, e.g. /usr/bin/php8.3.", SettingPHPBinary
	case found.Directory && !found.Console:
		return Check{Name: CheckConfiguration, State: CheckFail, Summary: "not a Shopware project",
				Detail: "bin/console not found in " + cfg.Directory},
			"Edit Directory and point it at the project root on the remote host.", SettingDirectory
	default:
		return Check{}, "", ""
	}
}

// classifySSHError turns the ssh client's message into a short reason and a
// hint that names the setting to change.
func classifySSHError(message string, cfg *shop.EnvironmentSSHConfig) (string, string, Setting) {
	lower := strings.ToLower(message)
	port := cfg.Port
	if port == 0 {
		port = 22
	}

	switch {
	case strings.Contains(lower, "connection refused"):
		hint := fmt.Sprintf("Check Port %d and Host, and that the server accepts SSH.", port)
		if port == 22 {
			hint = "Port is the default 22. If your server uses another port, edit Port. Otherwise check the host, and that the server accepts SSH."
		}
		return fmt.Sprintf("connection refused on port %d", port), hint, SettingPort
	case strings.Contains(lower, "could not resolve hostname"), strings.Contains(lower, "name or service not known"):
		return "host not found", fmt.Sprintf("%q does not resolve. Edit Host.", cfg.Host), SettingHost
	case strings.Contains(lower, "timed out"):
		return "connection timed out", "Check Host and Port, and that a firewall or VPN is not blocking the connection.", SettingHost
	case strings.Contains(lower, "host key verification failed"):
		return "host key not trusted", fmt.Sprintf("Run \"ssh %s\" once to review and accept the host key, then test again.", cfg.Host), SettingHost
	case strings.Contains(lower, "permission denied"), strings.Contains(lower, "too many authentication failures"):
		return "permission denied", "Check User and Identity file. The test does not prompt, so a key with a passphrase has to be loaded in the ssh agent.", SettingUser
	}

	lines := strings.Split(strings.TrimSpace(message), "\n")
	reason := strings.TrimSpace(lines[len(lines)-1])
	if reason == "" {
		reason = "connection failed"
	}
	return reason, "Check Host, User and Port.", SettingHost
}
